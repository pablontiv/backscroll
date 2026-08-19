package models

// IndexedRecord represents a single canonical record from search_items.
type IndexedRecord struct {
	Source      string
	SourcePath  string
	Ordinal     int64
	Role        string
	Text        string
	Project     *string
	UUID        *string
	Timestamp   *string
	ContentType string
}
