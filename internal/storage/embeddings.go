package storage

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ChunkEmbedding holds a chunk ID, its source search_item ID, and its embedding vector.
type ChunkEmbedding struct {
	ChunkID   int64
	ItemID    int64
	Embedding []float32
}

// VectorResult is a search result ranked by cosine similarity.
type VectorResult struct {
	ItemID     int64
	Similarity float64
}

// encodeEmbedding serializes float32 slice to little-endian bytes.
func encodeEmbedding(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeEmbedding deserializes little-endian bytes to float32 slice.
func decodeEmbedding(buf []byte) []float32 {
	if len(buf)%4 != 0 {
		return nil
	}
	v := make([]float32, len(buf)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return v
}

// cosineSimilarity computes cosine similarity between two equal-length vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float32) float64 {
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

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

// GetVectorCount returns the number of chunks that have an embedding vector stored.
func (d *Database) GetVectorCount() (int, error) {
	var n int
	err := d.db.QueryRow("SELECT COUNT(*) FROM chunks WHERE embedding IS NOT NULL").Scan(&n)
	return n, err
}

// InsertChunkEmbedding stores the embedding vector for a chunk.
func (d *Database) InsertChunkEmbedding(chunkID int64, embedding []float32) error {
	_, err := d.db.Exec(
		"UPDATE chunks SET embedding = ? WHERE id = ?",
		encodeEmbedding(embedding), chunkID,
	)
	return err
}

// LoadChunkEmbeddings returns all chunks that have an embedding, joined to their
// search_items row via source_id = source_path. Used for linear-scan vector search.
func (d *Database) LoadChunkEmbeddings() ([]ChunkEmbedding, error) {
	rows, err := d.db.Query(`
		SELECT c.id, si.id, c.embedding
		FROM chunks c
		JOIN search_items si ON si.source_path = c.source_id
		WHERE c.embedding IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("load chunk embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []ChunkEmbedding
	for rows.Next() {
		var ce ChunkEmbedding
		var blob []byte
		if err := rows.Scan(&ce.ChunkID, &ce.ItemID, &blob); err != nil {
			return nil, fmt.Errorf("scan chunk embedding: %w", err)
		}
		ce.Embedding = decodeEmbedding(blob)
		results = append(results, ce)
	}
	return results, rows.Err()
}

// VectorSearch performs a linear-scan cosine similarity search over all stored embeddings.
// Returns up to topK results sorted by descending similarity.
func (d *Database) VectorSearch(queryVec []float32, topK int) ([]VectorResult, error) {
	chunks, err := d.LoadChunkEmbeddings()
	if err != nil {
		return nil, err
	}

	// Deduplicate by ItemID, keep max similarity per item
	best := make(map[int64]float64, len(chunks))
	for _, c := range chunks {
		if len(c.Embedding) != len(queryVec) {
			continue
		}
		sim := cosineSimilarity(queryVec, c.Embedding)
		if sim > best[c.ItemID] {
			best[c.ItemID] = sim
		}
	}

	results := make([]VectorResult, 0, len(best))
	for itemID, sim := range best {
		results = append(results, VectorResult{ItemID: itemID, Similarity: sim})
	}

	// Sort descending by similarity
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Similarity > results[j-1].Similarity; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}
