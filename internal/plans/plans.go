package plans

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PlanSection represents a section of a markdown plan file.
type PlanSection struct {
	Title   string
	Content string // full section content including title
	Source  string // always "plan"
}

// ParsePlan parses a markdown file and returns one PlanSection per ## header.
// If no ## headers are found, the entire file is returned as a single section.
func ParsePlan(path string) ([]PlanSection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	var sections []PlanSection
	var currentSection *PlanSection
	var currentContent strings.Builder

	for _, line := range lines {
		// Check if this line starts a new section with ##
		if strings.HasPrefix(line, "## ") {
			// Save previous section if any
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(currentContent.String())
				sections = append(sections, *currentSection)
				currentContent.Reset()
			}

			// Start new section
			title := strings.TrimPrefix(line, "## ")
			title = strings.TrimSpace(title)
			currentSection = &PlanSection{
				Title:   title,
				Content: "", // will be populated
				Source:  "plan",
			}
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		} else if currentSection != nil {
			// Add to current section
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}

	// Save last section if any
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(currentContent.String())
		sections = append(sections, *currentSection)
	}

	// If no sections were found (no ## headers), treat entire file as one section
	if len(sections) == 0 {
		sections = []PlanSection{
			{
				Title:   "Untitled",
				Content: strings.TrimSpace(content),
				Source:  "plan",
			},
		}
	}

	return sections, nil
}

// DiscoverPlanFiles returns all .md files in the given directory.
func DiscoverPlanFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}
