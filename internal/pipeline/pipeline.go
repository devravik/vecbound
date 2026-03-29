package pipeline

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/devravik/vecbound/internal/embedder"
	"github.com/devravik/vecbound/internal/processor"
	"github.com/devravik/vecbound/internal/storage"
)

// EmbeddedChunk holds a chunk's text and its vector embedding.
type EmbeddedChunk struct {
	Content string
	Index   int
	Vector  []float32
}

// ChunkedFile represents a file that has been chunked and embedded.
type ChunkedFile struct {
	Path   string
	Chunks []EmbeddedChunk
}

// Result holds the output of the full pipeline for a single file.
type Result struct {
	Path       string
	ChunkCount int
	Embedded   bool
}

// Run executes the crawl-chunk-embed-storage pipeline.
// If emb is nil, it skips the embedding step.
func Run(source string, workers int, chunkSize int, chunkOverlap int, maxMem int, emb embedder.Embedder, db *storage.DB) ([]Result, error) {
	walker := processor.NewWalker()
	chunker := processor.NewChunker(chunkSize, chunkOverlap)

	// Set memory limit on DB if provided
	if db != nil {
		db.MaxMemMB = maxMem
	}

	files, errc := walker.Walk(source)

	// Fan-out: multiple workers chunk and embed files in parallel
	chunked := make(chan ChunkedFile)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for file := range files {
				slog.Debug("worker processing", "worker", id, "path", file.Path)
				rawChunks := chunker.ChunkText(file.Content)
				if len(rawChunks) == 0 {
					continue
				}

				embedded := make([]EmbeddedChunk, len(rawChunks))
				for j, rc := range rawChunks {
					ec := EmbeddedChunk{
						Content: rc.Content,
						Index:   rc.Index,
					}

					// Embed if embedder is available
					if emb != nil {
						vec, err := emb.Embed(rc.Content)
						if err != nil {
							slog.Error("embedding failed",
								"path", file.Path,
								"chunk", rc.Index,
								"error", err,
							)
						} else {
							ec.Vector = vec
						}
					}

					embedded[j] = ec
				}

				chunked <- ChunkedFile{
					Path:   file.Path,
					Chunks: embedded,
				}
			}
		}(i)
	}

	// Close chunked channel when all workers finish
	go func() {
		wg.Wait()
		close(chunked)
	}()

	// Collect results and save to storage
	var results []Result
	for cf := range chunked {
		hasVectors := len(cf.Chunks) > 0 && cf.Chunks[0].Vector != nil

		// Save to DB if provided
		if db != nil {
			err := func() error {
				tx, err := db.BeginTx()
				if err != nil {
					return fmt.Errorf("failed to start transaction: %w", err)
				}
				defer tx.Rollback()

				docID, err := db.SaveDocument(cf.Path, filepath.Base(cf.Path))
				if err != nil {
					return fmt.Errorf("failed to save document: %w", err)
				}

				for _, ec := range cf.Chunks {
					chunkID, err := db.SaveChunkTx(tx, docID, ec.Index, ec.Content)
					if err != nil {
						return fmt.Errorf("failed to save chunk: %w", err)
					}
					if hasVectors && ec.Vector != nil {
						if err := db.SaveVectorTx(tx, chunkID, ec.Vector); err != nil {
							return fmt.Errorf("failed to save vector: %w", err)
						}
					}
				}
				return tx.Commit()
			}()
			
			if err != nil {
				slog.Error("failed to index file", "path", cf.Path, "error", err)
			}
			
			// Hint GC if we hit memory limits after this file
			db.CheckMemoryLimit()
		}

		slog.Info("processed file",
			"path", cf.Path,
			"chunks", len(cf.Chunks),
			"embedded", hasVectors,
		)
		results = append(results, Result{
			Path:       cf.Path,
			ChunkCount: len(cf.Chunks),
			Embedded:   hasVectors,
		})
	}

	// Check for walk errors
	if err := <-errc; err != nil {
		return results, err
	}

	return results, nil
}
