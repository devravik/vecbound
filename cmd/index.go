package cmd

import (
	"fmt"
	"log/slog"

	"github.com/devravik/vecbound/internal/embedder"
	"github.com/devravik/vecbound/internal/pipeline"
	"github.com/devravik/vecbound/internal/storage"
	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index a directory into a vectorized SQLite database",
	Long: `Crawl a source directory, chunk text files, generate embeddings
using a local ONNX model, and store results in a SQLite vector database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		source, _ := cmd.Flags().GetString("source")
		out, _ := cmd.Flags().GetString("out")
		workers, _ := cmd.Flags().GetInt("workers")
		runtimePath, _ := cmd.Flags().GetString("runtime")
		modelPath, _ := cmd.Flags().GetString("model")
		vocabPath, _ := cmd.Flags().GetString("vocab")
		chunkSize, _ := cmd.Flags().GetInt("chunk-size")
		chunkOverlap, _ := cmd.Flags().GetInt("chunk-overlap")

		if source == "" {
			return fmt.Errorf("--source is required")
		}
		if out == "" {
			out = "vec.db"
		}
		if workers <= 0 {
			workers = Cfg.Workers
		}

		slog.Info("indexing",
			"source", source,
			"out", out,
			"workers", workers,
			"chunk_size", Cfg.ChunkSize,
			"chunk_overlap", Cfg.ChunkOverlap,
		)

		// Initialize ONNX embedder
		emb, err := embedder.NewONNXEmbedder(runtimePath, modelPath, vocabPath)
		if err != nil {
			return fmt.Errorf("failed to initialize embedder: %w", err)
		}
		defer emb.Close()
		slog.Debug("ONNX model loaded")

		// Initialize Database
		slog.Info("initializing database", "path", out)
		db, err := storage.NewDB(out)
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer db.Close()

		clear, _ := cmd.Flags().GetBool("clear")
		if clear {
			slog.Info("clearing existing index", "path", out)
			if err := db.TruncateDB(); err != nil {
				return fmt.Errorf("failed to clear database: %w", err)
			}
		}

		results, err := pipeline.Run(source, workers, chunkSize, chunkOverlap, Cfg.MaxMem, emb, db)
		if err != nil {
			return fmt.Errorf("pipeline error: %w", err)
		}

		totalChunks := 0
		embeddedCount := 0
		for _, r := range results {
			totalChunks += r.ChunkCount
			if r.Embedded {
				embeddedCount++
			}
		}

		fmt.Printf("✅ Indexed %d files (%d chunks, %d embedded) -> %s\n",
			len(results), totalChunks, embeddedCount, out)
		return nil
	},
}

func init() {
	indexCmd.Flags().StringP("source", "s", "", "source directory to index")
	indexCmd.Flags().StringP("out", "o", "vec.db", "output database file")
	indexCmd.Flags().IntP("workers", "w", 4, "number of concurrent workers")
	indexCmd.Flags().Bool("clear", false, "clear the existing index before indexing")
	indexCmd.Flags().String("runtime", "", "path to libonnxruntime.so")
	indexCmd.Flags().String("model", "model.onnx", "path to the ONNX model file")
	indexCmd.Flags().String("vocab", "vocab.txt", "path to the tokenizer vocabulary file")
	indexCmd.Flags().Int("chunk-size", 500, "number of tokens per chunk")
	indexCmd.Flags().Int("chunk-overlap", 50, "number of overlapping tokens between chunks")

	rootCmd.AddCommand(indexCmd)
}
