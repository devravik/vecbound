package storage

import (
	"os"
	"testing"
)

func TestSQLite(t *testing.T) {
	dbPath := "test_storage.db"
	defer os.Remove(dbPath)

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test SaveDocument
	docID, err := db.SaveDocument("test/path.txt", "path.txt")
	if err != nil {
		t.Fatalf("Failed to save document: %v", err)
	}
	if docID == 0 {
		t.Fatal("Expected non-zero document ID")
	}

	// Test SaveChunk
	chunkID, err := db.SaveChunk(docID, 0, "Hello, world!")
	if err != nil {
		t.Fatalf("Failed to save chunk: %v", err)
	}
	if chunkID == 0 {
		t.Fatal("Expected non-zero chunk ID")
	}

	// Test SaveVector
	vec := []float32{0.1, 0.2, 0.3}
	err = db.SaveVector(chunkID, vec)
	if err != nil {
		t.Fatalf("Failed to save vector: %v", err)
	}

	// Verify data
	var count int
	err = db.conn.QueryRow("SELECT count(*) FROM documents").Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("Document count mismatch: %v, %d", err, count)
	}

	err = db.conn.QueryRow("SELECT count(*) FROM chunks").Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("Chunk count mismatch: %v, %d", err, count)
	}

	err = db.conn.QueryRow("SELECT count(*) FROM vectors").Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("Vector count mismatch: %v, %d", err, count)
	}
}
