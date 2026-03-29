package storage

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"
)

// DB represents the SQLite storage engine.
type DB struct {
	conn     *sql.DB
	MaxMemMB int
}

// SearchResult represents a single match from a vector search.
type SearchResult struct {
	Path    string
	Content string
	Index   int
	Score   float32
}

// VectorEntry holds a chunk's ID and its embedding for matching.
type VectorEntry struct {
	ChunkID   int64
	Embedding []float32
	Content   string
}

// NewDB opens (or creates) a SQLite database at the specified path.
func NewDB(dbPath string) (*DB, error) {
	// Ensure the parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	// Use connection string to ensure pragmas are applied to every connection
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Manual one-time cleanup of existing orphaned vectors from previous bug
	if _, err := conn.Exec("DELETE FROM vectors WHERE chunk_id NOT IN (SELECT id FROM chunks)"); err != nil {
		slog.Error("failed to clean orphaned vectors", "error", err)
	}

	// Register custom cosine_similarity function (if supported by the driver)
	// modernc/sqlite supports registering Go functions via the math/sql driver
	// but we'll implement it as a normal Go function for Phase 5 search.

	return db, nil
}

func (db *DB) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT UNIQUE NOT NULL,
			filename TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			document_id INTEGER NOT NULL,
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			FOREIGN KEY(document_id) REFERENCES documents(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS vectors (
			chunk_id INTEGER PRIMARY KEY,
			embedding BLOB NOT NULL,
			FOREIGN KEY(chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
		)`,
	}

	for _, q := range queries {
		if _, err := db.conn.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// TruncateDB clears all data from the database.
func (db *DB) TruncateDB() error {
	tables := []string{"vectors", "chunks", "documents"}
	for _, table := range tables {
		if _, err := db.conn.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to truncate %s: %w", table, err)
		}
	}
	// Reset autoincrement sequences
	_, err := db.conn.Exec("DELETE FROM sqlite_sequence")
	return err
}

// SaveDocument inserts a document and returns its ID.
// It deletes any existing document with the same path to ensure all related
// chunks and vectors are purged via ON DELETE CASCADE.
func (db *DB) SaveDocument(path, filename string) (int64, error) {
	var id int64
	// Check if document exists
	err := db.conn.QueryRow("SELECT id FROM documents WHERE path = ?", path).Scan(&id)
	if err == nil {
		// Exists, delete it (cascades to chunks and vectors)
		if _, err := db.conn.Exec("DELETE FROM documents WHERE id = ?", id); err != nil {
			return 0, fmt.Errorf("failed to delete existing document: %w", err)
		}
	}

	// Insert fresh
	err = db.conn.QueryRow(
		"INSERT INTO documents (path, filename) VALUES (?, ?) RETURNING id",
		path, filename,
	).Scan(&id)
	return id, err
}

// SaveChunkTx inserts a chunk using an existing transaction.
func (db *DB) SaveChunkTx(tx *sql.Tx, docID int64, index int, content string) (int64, error) {
	var id int64
	err := tx.QueryRow(
		"INSERT INTO chunks (document_id, chunk_index, content) VALUES (?, ?, ?) RETURNING id",
		docID, index, content,
	).Scan(&id)
	return id, err
}

// SaveVectorTx inserts a vector using an existing transaction.
func (db *DB) SaveVectorTx(tx *sql.Tx, chunkID int64, embedding []float32) error {
	blob := float32ToByte(embedding)
	_, err := tx.Exec("INSERT INTO vectors (chunk_id, embedding) VALUES (?, ?)", chunkID, blob)
	return err
}

// SaveChunk inserts a chunk (non-transactional).
func (db *DB) SaveChunk(docID int64, index int, content string) (int64, error) {
	var id int64
	err := db.conn.QueryRow(
		"INSERT INTO chunks (document_id, chunk_index, content) VALUES (?, ?, ?) RETURNING id",
		docID, index, content,
	).Scan(&id)
	return id, err
}

// SaveVector inserts a vector (non-transactional).
func (db *DB) SaveVector(chunkID int64, embedding []float32) error {
	blob := float32ToByte(embedding)
	_, err := db.conn.Exec("INSERT INTO vectors (chunk_id, embedding) VALUES (?, ?)", chunkID, blob)
	return err
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// CheckMemoryLimit checks if the current heap allocation exceeds the soft limit
// and hints the GC to run if necessary.
func (db *DB) CheckMemoryLimit() {
	if db.MaxMemMB <= 0 {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Alloc > uint64(db.MaxMemMB)*1024*1024 {
		slog.Debug("memory limit reached, forcing garbage collection",
			"alloc_mb", m.Alloc/1024/1024,
			"limit_mb", db.MaxMemMB,
		)
		runtime.GC()
	}
}

// BeginTx starts a transaction.
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// GetAllVectors returns all stored vectors along with chunk content for matching.
func (db *DB) GetAllVectors() ([]VectorEntry, error) {
	rows, err := db.conn.Query(`
		SELECT v.chunk_id, v.embedding, c.content 
		FROM vectors v
		JOIN chunks c ON v.chunk_id = c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []VectorEntry
	for rows.Next() {
		var chunkID int64
		var embeddingBlob []byte
		var content string
		if err := rows.Scan(&chunkID, &embeddingBlob, &content); err != nil {
			return nil, err
		}
		entries = append(entries, VectorEntry{
			ChunkID:   chunkID,
			Embedding: byteToFloat32(embeddingBlob),
			Content:   content,
		})
	}
	return entries, nil
}

// GetChunkDetails returns the text content and file path for a chunk ID.
func (db *DB) GetChunkDetails(chunkID int64) (path string, content string, index int, err error) {
	err = db.conn.QueryRow(`
		SELECT d.path, c.content, c.chunk_index 
		FROM chunks c 
		JOIN documents d ON c.document_id = d.id 
		WHERE c.id = ?`,
		chunkID,
	).Scan(&path, &content, &index)
	return
}

// CosineSimilarity calculates the similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// Helper: Float32 slice to byte slice
func float32ToByte(f []float32) []byte {
	b := make([]byte, len(f)*4)
	for i, v := range f {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

// Helper: Byte slice to float32 slice
func byteToFloat32(b []byte) []float32 {
	f := make([]float32, len(b)/4)
	for i := range f {
		f[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return f
}
