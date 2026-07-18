package sequences

// Sequence represents one session's ordered list of category symbols.
type Sequence struct {
	SessionID string
	Items     []string
}

// Pattern represents a discovered frequent subsequence.
type Pattern struct {
	Items   []string
	Support int // count of distinct sessions containing this pattern
}
