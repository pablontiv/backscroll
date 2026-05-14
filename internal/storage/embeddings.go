package storage

import "fmt"

// ChunkRecord represents a text chunk ready for embedding.
type ChunkRecord struct {
	ChunkIdx   int
	Content    string
	TokenCount int
}

// InsertChunks replaces all chunks for sourcePath and inserts the new set.
// Returns the inserted chunk IDs in order.
func (d *Database) InsertChunks(sourcePath string, chunks []ChunkRecord, createdAt int64) ([]int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM chunks WHERE source_id = ?", sourcePath); err != nil {
		return nil, fmt.Errorf("delete old chunks: %w", err)
	}

	ids := make([]int64, 0, len(chunks))
	for _, c := range chunks {
		res, err := tx.Exec(
			`INSERT INTO chunks (source_id, chunk_idx, content, token_count, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			sourcePath, c.ChunkIdx, c.Content, c.TokenCount, createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert chunk %d: %w", c.ChunkIdx, err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit chunks: %w", err)
	}
	return ids, nil
}

// InsertEmbeddingMetadata records metadata for a generated embedding.
func (d *Database) InsertEmbeddingMetadata(chunkID int64, modelName, modelVersion string, dimensions int, createdAt int64) error {
	_, err := d.db.Exec(
		`INSERT INTO embedding_metadata (chunk_id, model_name, model_version, dimensions, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		chunkID, modelName, modelVersion, dimensions, createdAt,
	)
	return err
}

// GetChunkCount returns the total number of stored chunks.
func (d *Database) GetChunkCount() (int, error) {
	var n int
	err := d.db.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&n)
	return n, err
}

// GetEmbeddingCount returns the total number of stored embedding metadata rows.
func (d *Database) GetEmbeddingCount() (int, error) {
	var n int
	err := d.db.QueryRow("SELECT COUNT(*) FROM embedding_metadata").Scan(&n)
	return n, err
}
