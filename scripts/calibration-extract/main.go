package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sort"

	"github.com/pablontiv/backscroll/internal/storage"
)

func main() {
	total := flag.Int("total", 50, "total samples to extract")
	perDetector := flag.Int("per-detector", 0, "samples per detector (0 = auto-equal)")
	perSession := flag.Int("per-session", 10, "max samples per session")
	output := flag.String("output", "", "output CSV path (REQUIRED; must be outside repo)")
	flag.Parse()

	if *total <= 0 {
		log.Fatal("--total must be > 0")
	}
	if *perSession <= 0 {
		log.Fatal("--per-session must be > 0")
	}

	// Require explicit --output to prevent committing private session text
	if *output == "" {
		fmt.Fprintf(os.Stderr, "missing --output; write the worksheet OUTSIDE the repository\n")
		fmt.Fprintf(os.Stderr, "(private session text must not be committed)\n")
		fmt.Fprintf(os.Stderr, "Example: --output ~/calibration/corrections-labeling-2026-07-20.csv\n")
		os.Exit(1)
	}

	// Open DB read-only
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("get current user: %v", err)
	}
	dbPath := filepath.Join(currentUser.HomeDir, ".backscroll.db")
	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Query corrections with min_confidence=0.4
	opts := storage.CorrectionAggOpts{
		MinConfidence: 0.4,
		Limit:         0, // no limit at query time; paginate mined patterns only
	}
	candidates, err := db.AggregateCorrections(opts)
	if err != nil {
		log.Fatalf("query corrections: %v", err)
	}

	// Stratified sampling with window context population
	samples := stratifyWithDB(db, candidates, *total, *perDetector, *perSession)

	// Output CSV
	if err := writeCSV(*output, samples); err != nil {
		log.Fatalf("write csv: %v", err)
	}

	fmt.Printf("Extracted %d samples to %s\n", len(samples), *output)
}

// Sample represents one row in the output CSV.
type Sample struct {
	UUID         string
	Kind         string
	Label        string
	Detector     string
	Confidence   float64
	SourcePath   string
	Ordinal      int
	SessionTag   string
	TextPreview  string
	WindowBefore string
	WindowAfter  string
}

// stratify applies per-detector and per-session quotas and returns deterministically ordered samples.
// getContextMessage fetches a message from the database with specific constraints.
// If before=true, fetches the LAST message with role='assistant' before ordinal,
// skipping tool rows and stubs < 20 runes.
// If before=false, fetches the NEXT message with role='user' after ordinal,
// skipping tool rows and stubs < 20 runes.
func getContextMessage(db *storage.Database, sourcePath string, ordinal int, before bool) string {
	var query string
	var result string

	if before {
		// Last assistant message before this ordinal, skip tool rows and short stubs
		query = `
			SELECT text FROM search_items
			WHERE source_path = ? AND ordinal < ? AND role = 'assistant' AND content_type != 'tool'
			  AND length(CAST(text AS TEXT)) >= 20
			ORDER BY ordinal DESC
			LIMIT 1
		`
	} else {
		// Next user message after this ordinal, skip tool rows and short stubs
		query = `
			SELECT text FROM search_items
			WHERE source_path = ? AND ordinal > ? AND role = 'user' AND content_type != 'tool'
			  AND length(CAST(text AS TEXT)) >= 20
			ORDER BY ordinal ASC
			LIMIT 1
		`
	}

	err := db.DB().QueryRow(query, sourcePath, ordinal).Scan(&result)
	if err != nil {
		// No matching message found
		return ""
	}

	// Truncate to 50 chars to match TextPreview cap
	if len(result) > 50 {
		return result[:50]
	}
	return result
}

// stratifyWithDB applies stratification and populates window context from the database.
func stratifyWithDB(db *storage.Database, candidates []storage.CorrectionCandidate, total int, perDetectorFlag int, perSessionCap int) []Sample {
	samples := stratify(candidates, total, perDetectorFlag, perSessionCap)

	// Populate window context for each sample
	for i := range samples {
		samples[i].WindowBefore = getContextMessage(db, samples[i].SourcePath, samples[i].Ordinal, true)
		samples[i].WindowAfter = getContextMessage(db, samples[i].SourcePath, samples[i].Ordinal, false)
	}

	return samples
}

func stratify(candidates []storage.CorrectionCandidate, total int, perDetectorFlag int, perSessionCap int) []Sample {
	// Group by detector
	byDetector := make(map[string][]storage.CorrectionCandidate)
	for _, c := range candidates {
		for _, d := range c.Detectors {
			byDetector[d] = append(byDetector[d], c)
		}
	}

	// Determine per-detector quota
	var perDetector int
	if perDetectorFlag > 0 {
		perDetector = perDetectorFlag
	} else {
		// Auto-equal: split total evenly among detectors
		if len(byDetector) > 0 {
			perDetector = total / len(byDetector)
		} else {
			return nil
		}
	}

	var samples []Sample
	detectorNames := make([]string, 0, len(byDetector))
	for d := range byDetector {
		detectorNames = append(detectorNames, d)
	}
	sort.Strings(detectorNames) // deterministic order

	shortfallReport := make(map[string]int) // detector -> shortfall count

	for _, detectorName := range detectorNames {
		detCands := byDetector[detectorName]
		taken := 0

		// Group by session for this detector
		bySession := make(map[string][]storage.CorrectionCandidate)
		for _, c := range detCands {
			bySession[c.SourcePath] = append(bySession[c.SourcePath], c)
		}

		// Deterministic session order
		sessions := make([]string, 0, len(bySession))
		for s := range bySession {
			sessions = append(sessions, s)
		}
		sort.Strings(sessions)

		// Apply per-session cap and per-detector quota
		for _, session := range sessions {
			sessionCands := bySession[session]
			// Take up to perSessionCap from this session
			limit := perSessionCap
			if len(sessionCands) < limit {
				limit = len(sessionCands)
			}

			for i := 0; i < limit && taken < perDetector; i++ {
				c := sessionCands[i]
				sample := Sample{
					UUID:       c.UUID,
					Kind:       "correction",
					Label:      "",
					Detector:   detectorName,
					Confidence: c.MaxConfidence,
					SourcePath: c.SourcePath,
					Ordinal:    c.Ordinal,
					SessionTag: "", // populated by caller if needed
				}

				// Text preview (first 50 chars)
				if len(c.TextSnippet) > 50 {
					sample.TextPreview = c.TextSnippet[:50]
				} else {
					sample.TextPreview = c.TextSnippet
				}

				samples = append(samples, sample)
				taken++
			}
		}

		// Report shortfall
		if taken < perDetector {
			shortfallReport[detectorName] = perDetector - taken
		}
	}

	// Emit shortfall report to stderr
	for detectorName, shortfall := range shortfallReport {
		fmt.Fprintf(os.Stderr, "Requested %d %s, found %d\n", perDetector, detectorName, perDetector-shortfall)
	}

	return samples
}

// writeCSV writes samples to CSV file
func writeCSV(path string, samples []Sample) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = file.Close() }()

	w := csv.NewWriter(file)
	defer w.Flush()

	// Write header
	header := []string{"uuid", "kind", "label", "detector", "confidence", "source_path", "ordinal",
		"session_tag", "text_preview", "labeling_window_before", "labeling_window_after"}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write rows
	for _, s := range samples {
		row := []string{
			s.UUID,
			s.Kind,
			s.Label,
			s.Detector,
			fmt.Sprintf("%.1f", s.Confidence),
			s.SourcePath,
			fmt.Sprintf("%d", s.Ordinal),
			s.SessionTag,
			s.TextPreview,
			s.WindowBefore,
			s.WindowAfter,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}

	return nil
}
