package models

import (
	"context"
	"testing"
	"time"
)

// TestSearchResultCreation tests SearchResult struct creation and field access.
func TestSearchResultCreation(t *testing.T) {
	timestamp := time.Date(2024, 5, 14, 10, 30, 0, 0, time.UTC)
	sr := SearchResult{
		Source:      "session",
		Role:        "assistant",
		Content:     "test content",
		FilePath:    "/path/to/file",
		Timestamp:   timestamp,
		SessionID:   "session-123",
		ProjectPath: "/home/user/project",
		Score:       0.85,
		Tags:        []string{"feature", "testing"},
		ContentType: "text",
		Rank:        1,
	}

	if sr.Source != "session" {
		t.Errorf("expected Source 'session', got %q", sr.Source)
	}
	if sr.Role != "assistant" {
		t.Errorf("expected Role 'assistant', got %q", sr.Role)
	}
	if sr.Content != "test content" {
		t.Errorf("expected Content 'test content', got %q", sr.Content)
	}
	if sr.FilePath != "/path/to/file" {
		t.Errorf("expected FilePath '/path/to/file', got %q", sr.FilePath)
	}
	if sr.Timestamp != timestamp {
		t.Errorf("expected Timestamp %v, got %v", timestamp, sr.Timestamp)
	}
	if sr.SessionID != "session-123" {
		t.Errorf("expected SessionID 'session-123', got %q", sr.SessionID)
	}
	if sr.ProjectPath != "/home/user/project" {
		t.Errorf("expected ProjectPath '/home/user/project', got %q", sr.ProjectPath)
	}
	if sr.Score != 0.85 {
		t.Errorf("expected Score 0.85, got %f", sr.Score)
	}
	if len(sr.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(sr.Tags))
	}
	if sr.Tags[0] != "feature" || sr.Tags[1] != "testing" {
		t.Errorf("expected tags [feature, testing], got %v", sr.Tags)
	}
	if sr.ContentType != "text" {
		t.Errorf("expected ContentType 'text', got %q", sr.ContentType)
	}
	if sr.Rank != 1 {
		t.Errorf("expected Rank 1, got %d", sr.Rank)
	}
}

// TestSearchResultDifferentSources tests SearchResult with different source values.
func TestSearchResultDifferentSources(t *testing.T) {
	sources := []string{"session", "plan", "ke", "decision", "memory", "rule", "spec", "backlog"}
	for _, source := range sources {
		sr := SearchResult{
			Source: source,
			Role:   "user",
		}
		if sr.Source != source {
			t.Errorf("expected Source %q, got %q", source, sr.Source)
		}
	}
}

// TestSearchResultDifferentRoles tests SearchResult with different role values.
func TestSearchResultDifferentRoles(t *testing.T) {
	roles := []string{"user", "assistant", "system"}
	for _, role := range roles {
		sr := SearchResult{
			Role:   role,
			Source: "session",
		}
		if sr.Role != role {
			t.Errorf("expected Role %q, got %q", role, sr.Role)
		}
	}
}

// TestSearchResultDifferentContentTypes tests SearchResult with different content types.
func TestSearchResultDifferentContentTypes(t *testing.T) {
	contentTypes := []string{"text", "code", "tool"}
	for _, ct := range contentTypes {
		sr := SearchResult{
			ContentType: ct,
			Source:      "session",
		}
		if sr.ContentType != ct {
			t.Errorf("expected ContentType %q, got %q", ct, sr.ContentType)
		}
	}
}

// TestSearchResultZeroValues tests SearchResult with zero values.
func TestSearchResultZeroValues(t *testing.T) {
	sr := SearchResult{}
	if sr.Source != "" {
		t.Errorf("expected empty Source, got %q", sr.Source)
	}
	if sr.Role != "" {
		t.Errorf("expected empty Role, got %q", sr.Role)
	}
	if sr.Score != 0.0 {
		t.Errorf("expected zero Score, got %f", sr.Score)
	}
	if sr.Rank != 0 {
		t.Errorf("expected zero Rank, got %d", sr.Rank)
	}
	if sr.Tags != nil {
		t.Errorf("expected nil Tags, got %v", sr.Tags)
	}
}

// TestSearchResultEmptyTags tests SearchResult with empty tags slice.
func TestSearchResultEmptyTags(t *testing.T) {
	sr := SearchResult{
		Tags:   []string{},
		Source: "session",
	}
	if len(sr.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(sr.Tags))
	}
}

// TestSearchResultMultipleTags tests SearchResult with multiple tags.
func TestSearchResultMultipleTags(t *testing.T) {
	tags := []string{"debugging", "refactoring", "feature", "testing", "docs", "config"}
	sr := SearchResult{
		Tags:   tags,
		Source: "session",
	}
	if len(sr.Tags) != len(tags) {
		t.Errorf("expected %d tags, got %d", len(tags), len(sr.Tags))
	}
	for i, tag := range tags {
		if sr.Tags[i] != tag {
			t.Errorf("tag[%d]: expected %q, got %q", i, tag, sr.Tags[i])
		}
	}
}

// TestSearchResultScoreRange tests SearchResult with various score values.
func TestSearchResultScoreRange(t *testing.T) {
	scores := []float64{0.0, 0.5, 0.85, 1.0, 2.5}
	for _, score := range scores {
		sr := SearchResult{
			Score:  score,
			Source: "session",
		}
		if sr.Score != score {
			t.Errorf("expected Score %f, got %f", score, sr.Score)
		}
	}
}

// TestParsedFileCreation tests ParsedFile struct creation and field access.
func TestParsedFileCreation(t *testing.T) {
	timestamp := time.Date(2024, 5, 14, 10, 30, 0, 0, time.UTC)
	messages := []Message{
		{
			Role:        "user",
			Content:     "question",
			ContentType: "text",
			Timestamp:   timestamp,
		},
		{
			Role:        "assistant",
			Content:     "answer",
			ContentType: "text",
			Timestamp:   timestamp.Add(1 * time.Second),
		},
	}

	pf := ParsedFile{
		Path:    "/path/to/session.jsonl",
		Hash:    "abc123def456",
		Records: messages,
	}

	if pf.Path != "/path/to/session.jsonl" {
		t.Errorf("expected Path '/path/to/session.jsonl', got %q", pf.Path)
	}
	if pf.Hash != "abc123def456" {
		t.Errorf("expected Hash 'abc123def456', got %q", pf.Hash)
	}
	if len(pf.Records) != 2 {
		t.Errorf("expected 2 records, got %d", len(pf.Records))
	}
	if pf.Records[0].Role != "user" {
		t.Errorf("expected first record role 'user', got %q", pf.Records[0].Role)
	}
	if pf.Records[1].Role != "assistant" {
		t.Errorf("expected second record role 'assistant', got %q", pf.Records[1].Role)
	}
}

// TestParsedFileEmptyRecords tests ParsedFile with no records.
func TestParsedFileEmptyRecords(t *testing.T) {
	pf := ParsedFile{
		Path:    "/path/to/file",
		Hash:    "hash123",
		Records: []Message{},
	}

	if len(pf.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(pf.Records))
	}
}

// TestParsedFileZeroValues tests ParsedFile with zero values.
func TestParsedFileZeroValues(t *testing.T) {
	pf := ParsedFile{}

	if pf.Path != "" {
		t.Errorf("expected empty Path, got %q", pf.Path)
	}
	if pf.Hash != "" {
		t.Errorf("expected empty Hash, got %q", pf.Hash)
	}
	if pf.Records != nil {
		t.Errorf("expected nil Records, got %v", pf.Records)
	}
}

// TestMessageCreation tests Message struct creation and field access.
func TestMessageCreation(t *testing.T) {
	timestamp := time.Date(2024, 5, 14, 10, 30, 0, 0, time.UTC)
	msg := Message{
		Role:        "user",
		Content:     "test message",
		ContentType: "text",
		Timestamp:   timestamp,
	}

	if msg.Role != "user" {
		t.Errorf("expected Role 'user', got %q", msg.Role)
	}
	if msg.Content != "test message" {
		t.Errorf("expected Content 'test message', got %q", msg.Content)
	}
	if msg.ContentType != "text" {
		t.Errorf("expected ContentType 'text', got %q", msg.ContentType)
	}
	if msg.Timestamp != timestamp {
		t.Errorf("expected Timestamp %v, got %v", timestamp, msg.Timestamp)
	}
}

// TestMessageDifferentRoles tests Message with different role values.
func TestMessageDifferentRoles(t *testing.T) {
	roles := []string{"user", "assistant", "system"}
	for _, role := range roles {
		msg := Message{
			Role: role,
		}
		if msg.Role != role {
			t.Errorf("expected Role %q, got %q", role, msg.Role)
		}
	}
}

// TestMessageDifferentContentTypes tests Message with different content types.
func TestMessageDifferentContentTypes(t *testing.T) {
	contentTypes := []string{"text", "code", "tool"}
	for _, ct := range contentTypes {
		msg := Message{
			ContentType: ct,
		}
		if msg.ContentType != ct {
			t.Errorf("expected ContentType %q, got %q", ct, msg.ContentType)
		}
	}
}

// TestMessageZeroValues tests Message with zero values.
func TestMessageZeroValues(t *testing.T) {
	msg := Message{}

	if msg.Role != "" {
		t.Errorf("expected empty Role, got %q", msg.Role)
	}
	if msg.Content != "" {
		t.Errorf("expected empty Content, got %q", msg.Content)
	}
	if msg.ContentType != "" {
		t.Errorf("expected empty ContentType, got %q", msg.ContentType)
	}
	if !msg.Timestamp.IsZero() {
		t.Errorf("expected zero Timestamp, got %v", msg.Timestamp)
	}
}

// TestMessageLargeContent tests Message with large content.
func TestMessageLargeContent(t *testing.T) {
	largeContent := ""
	for i := 0; i < 1000; i++ {
		largeContent += "line " + string(rune(i)) + "\n"
	}

	msg := Message{
		Role:    "assistant",
		Content: largeContent,
	}

	if len(msg.Content) == 0 {
		t.Errorf("expected non-empty Content")
	}
	if msg.Content != largeContent {
		t.Errorf("expected Content to be preserved")
	}
}

// TestStatsCreation tests Stats struct creation and field access.
func TestStatsCreation(t *testing.T) {
	now := time.Date(2024, 5, 14, 10, 30, 0, 0, time.UTC)
	stats := Stats{
		TotalFiles:    42,
		TotalMessages: 256,
		IndexedAt:     now,
	}

	if stats.TotalFiles != 42 {
		t.Errorf("expected TotalFiles 42, got %d", stats.TotalFiles)
	}
	if stats.TotalMessages != 256 {
		t.Errorf("expected TotalMessages 256, got %d", stats.TotalMessages)
	}
	if stats.IndexedAt != now {
		t.Errorf("expected IndexedAt %v, got %v", now, stats.IndexedAt)
	}
}

// TestStatsZeroValues tests Stats with zero values.
func TestStatsZeroValues(t *testing.T) {
	stats := Stats{}

	if stats.TotalFiles != 0 {
		t.Errorf("expected TotalFiles 0, got %d", stats.TotalFiles)
	}
	if stats.TotalMessages != 0 {
		t.Errorf("expected TotalMessages 0, got %d", stats.TotalMessages)
	}
	if !stats.IndexedAt.IsZero() {
		t.Errorf("expected zero IndexedAt, got %v", stats.IndexedAt)
	}
}

// TestStatsLargeNumbers tests Stats with large numbers.
func TestStatsLargeNumbers(t *testing.T) {
	stats := Stats{
		TotalFiles:    1000000,
		TotalMessages: 5000000,
	}

	if stats.TotalFiles != 1000000 {
		t.Errorf("expected TotalFiles 1000000, got %d", stats.TotalFiles)
	}
	if stats.TotalMessages != 5000000 {
		t.Errorf("expected TotalMessages 5000000, got %d", stats.TotalMessages)
	}
}

// TestSearchOptionsCreation tests SearchOptions struct creation and field access.
func TestSearchOptionsCreation(t *testing.T) {
	after := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	opts := SearchOptions{
		Project:             "myproject",
		AllProjects:         false,
		Source:              "session",
		After:               &after,
		Before:              &before,
		Role:                "assistant",
		Limit:               10,
		Offset:              5,
		ContentType:         "text",
		Tag:                 "feature",
		LexicalOnly:         true,
		SimilarityThreshold: 0.75,
	}

	if opts.Project != "myproject" {
		t.Errorf("expected Project 'myproject', got %q", opts.Project)
	}
	if opts.AllProjects != false {
		t.Errorf("expected AllProjects false, got %v", opts.AllProjects)
	}
	if opts.Source != "session" {
		t.Errorf("expected Source 'session', got %q", opts.Source)
	}
	if opts.After == nil || *opts.After != after {
		t.Errorf("expected After %v, got %v", after, opts.After)
	}
	if opts.Before == nil || *opts.Before != before {
		t.Errorf("expected Before %v, got %v", before, opts.Before)
	}
	if opts.Role != "assistant" {
		t.Errorf("expected Role 'assistant', got %q", opts.Role)
	}
	if opts.Limit != 10 {
		t.Errorf("expected Limit 10, got %d", opts.Limit)
	}
	if opts.Offset != 5 {
		t.Errorf("expected Offset 5, got %d", opts.Offset)
	}
	if opts.ContentType != "text" {
		t.Errorf("expected ContentType 'text', got %q", opts.ContentType)
	}
	if opts.Tag != "feature" {
		t.Errorf("expected Tag 'feature', got %q", opts.Tag)
	}
	if opts.LexicalOnly != true {
		t.Errorf("expected LexicalOnly true, got %v", opts.LexicalOnly)
	}
	if opts.SimilarityThreshold != 0.75 {
		t.Errorf("expected SimilarityThreshold 0.75, got %f", opts.SimilarityThreshold)
	}
}

// TestSearchOptionsZeroValues tests SearchOptions with zero values.
func TestSearchOptionsZeroValues(t *testing.T) {
	opts := SearchOptions{}

	if opts.Project != "" {
		t.Errorf("expected empty Project, got %q", opts.Project)
	}
	if opts.AllProjects != false {
		t.Errorf("expected AllProjects false, got %v", opts.AllProjects)
	}
	if opts.Source != "" {
		t.Errorf("expected empty Source, got %q", opts.Source)
	}
	if opts.After != nil {
		t.Errorf("expected nil After, got %v", opts.After)
	}
	if opts.Before != nil {
		t.Errorf("expected nil Before, got %v", opts.Before)
	}
	if opts.LexicalOnly != false {
		t.Errorf("expected LexicalOnly false, got %v", opts.LexicalOnly)
	}
}

// TestSearchOptionsAllProjects tests SearchOptions with AllProjects true.
func TestSearchOptionsAllProjects(t *testing.T) {
	opts := SearchOptions{
		AllProjects: true,
	}

	if opts.AllProjects != true {
		t.Errorf("expected AllProjects true, got %v", opts.AllProjects)
	}
}

// TestSearchOptionsVariousSources tests SearchOptions with various source values.
func TestSearchOptionsVariousSources(t *testing.T) {
	sources := []string{"session", "plan", "ke", "decision", "memory", "rule", "spec", "backlog"}
	for _, source := range sources {
		opts := SearchOptions{
			Source: source,
		}
		if opts.Source != source {
			t.Errorf("expected Source %q, got %q", source, opts.Source)
		}
	}
}

// TestSearchOptionsSimilarityThreshold tests SearchOptions with various similarity thresholds.
func TestSearchOptionsSimilarityThreshold(t *testing.T) {
	thresholds := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
	for _, threshold := range thresholds {
		opts := SearchOptions{
			SimilarityThreshold: threshold,
		}
		if opts.SimilarityThreshold != threshold {
			t.Errorf("expected SimilarityThreshold %f, got %f", threshold, opts.SimilarityThreshold)
		}
	}
}

// TestSyncOptionsCreation tests SyncOptions struct creation and field access.
func TestSyncOptionsCreation(t *testing.T) {
	opts := SyncOptions{
		IncludeAgents: true,
		NoPlans:       false,
	}

	if opts.IncludeAgents != true {
		t.Errorf("expected IncludeAgents true, got %v", opts.IncludeAgents)
	}
	if opts.NoPlans != false {
		t.Errorf("expected NoPlans false, got %v", opts.NoPlans)
	}
}

// TestSyncOptionsZeroValues tests SyncOptions with zero values.
func TestSyncOptionsZeroValues(t *testing.T) {
	opts := SyncOptions{}

	if opts.IncludeAgents != false {
		t.Errorf("expected IncludeAgents false, got %v", opts.IncludeAgents)
	}
	if opts.NoPlans != false {
		t.Errorf("expected NoPlans false, got %v", opts.NoPlans)
	}
}

// TestSyncOptionsVariations tests SyncOptions with various combinations.
func TestSyncOptionsVariations(t *testing.T) {
	variations := []SyncOptions{
		{IncludeAgents: true, NoPlans: true},
		{IncludeAgents: true, NoPlans: false},
		{IncludeAgents: false, NoPlans: true},
		{IncludeAgents: false, NoPlans: false},
	}

	for i, opts := range variations {
		t.Logf("variation %d: IncludeAgents=%v, NoPlans=%v", i, opts.IncludeAgents, opts.NoPlans)
	}
}

// TestSearchEngineInterface tests that SearchEngine interface is properly defined.
func TestSearchEngineInterface(t *testing.T) {
	var _ SearchEngine = (*mockSearchEngine)(nil)
}

// mockSearchEngine is a mock implementation of SearchEngine for testing.
type mockSearchEngine struct{}

func (m *mockSearchEngine) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	return []SearchResult{}, nil
}

func (m *mockSearchEngine) Sync(ctx context.Context, paths []string, opts SyncOptions) (Stats, error) {
	return Stats{}, nil
}

func (m *mockSearchEngine) Close() error {
	return nil
}

// TestSearchEngineSearchMethod tests the Search method signature.
func TestSearchEngineSearchMethod(t *testing.T) {
	ctx := context.Background()
	engine := &mockSearchEngine{}

	results, err := engine.Search(ctx, "test", SearchOptions{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from mock, got %d", len(results))
	}
}

// TestSearchEngineSyncMethod tests the Sync method signature.
func TestSearchEngineSyncMethod(t *testing.T) {
	ctx := context.Background()
	engine := &mockSearchEngine{}

	paths := []string{"/path1", "/path2"}
	stats, err := engine.Sync(ctx, paths, SyncOptions{})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if stats.TotalFiles != 0 {
		t.Errorf("expected 0 files from mock, got %d", stats.TotalFiles)
	}
}

// TestSearchEngineCloseMethod tests the Close method signature.
func TestSearchEngineCloseMethod(t *testing.T) {
	engine := &mockSearchEngine{}
	err := engine.Close()
	if err != nil {
		t.Errorf("expected no error from Close, got %v", err)
	}
}

// TestSearchResultWithSpecialCharacters tests SearchResult with special characters.
func TestSearchResultWithSpecialCharacters(t *testing.T) {
	sr := SearchResult{
		Content: "test with special chars: \n\t\r\\\"'",
		Source:  "session",
	}

	if sr.Content != "test with special chars: \n\t\r\\\"'" {
		t.Errorf("expected content with special chars to be preserved")
	}
}

// TestMessageWithSpecialCharacters tests Message with special characters.
func TestMessageWithSpecialCharacters(t *testing.T) {
	msg := Message{
		Content: "test with special chars: \n\t\r\\\"'",
		Role:    "user",
	}

	if msg.Content != "test with special chars: \n\t\r\\\"'" {
		t.Errorf("expected content with special chars to be preserved")
	}
}

// TestSearchOptionsWithNilDates tests SearchOptions with nil date pointers.
func TestSearchOptionsWithNilDates(t *testing.T) {
	opts := SearchOptions{
		After:  nil,
		Before: nil,
	}

	if opts.After != nil {
		t.Errorf("expected nil After, got %v", opts.After)
	}
	if opts.Before != nil {
		t.Errorf("expected nil Before, got %v", opts.Before)
	}
}

// TestSearchResultRankValues tests SearchResult with various rank values.
func TestSearchResultRankValues(t *testing.T) {
	ranks := []int{0, 1, 10, 100, 1000}
	for _, rank := range ranks {
		sr := SearchResult{
			Rank:   rank,
			Source: "session",
		}
		if sr.Rank != rank {
			t.Errorf("expected Rank %d, got %d", rank, sr.Rank)
		}
	}
}
