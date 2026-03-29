package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/devravik/vecbound/internal/embedder"
	"github.com/devravik/vecbound/internal/storage"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search a vectorized database with a natural language query",
	Long: `Embed your query using the same local ONNX model and perform a
cosine similarity search against the SQLite vector database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, _ := cmd.Flags().GetString("db")
		query, _ := cmd.Flags().GetString("query")
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		runtimePath, _ := cmd.Flags().GetString("runtime")
		modelPath, _ := cmd.Flags().GetString("model")
		vocabPath, _ := cmd.Flags().GetString("vocab")
		boost, _ := cmd.Flags().GetFloat32("boost")

		if dbPath == "" {
			return fmt.Errorf("--db is required")
		}
		if query == "" {
			return fmt.Errorf("--query is required")
		}

		slog.Debug("searching",
			"db", dbPath,
			"query", query,
			"limit", limit,
			"offset", offset,
			"boost", boost,
		)

		// 1. Initialize ONNX embedder
		emb, err := embedder.NewONNXEmbedder(runtimePath, modelPath, vocabPath)
		if err != nil {
			return fmt.Errorf("failed to initialize embedder: %w", err)
		}
		defer emb.Close()

		// 2. Embed query
		queryVec, err := emb.Embed(query)
		if err != nil {
			return fmt.Errorf("failed to embed query: %w", err)
		}

		// 3. Open DB
		db, err := storage.NewDB(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		// 4. Get all vectors
		entries, err := db.GetAllVectors()
		if err != nil {
			return fmt.Errorf("failed to fetch vectors: %w", err)
		}

		// 5. Calculate similarities
		type scoredMatch struct {
			chunkID int64
			score   float32
		}
		var matches []scoredMatch
		lowerQuery := strings.ToLower(query)
		for _, entry := range entries {
			score := storage.CosineSimilarity(queryVec, entry.Embedding)
			
			// Hybrid Search: Boost exact matches
			if boost > 0 && strings.Contains(strings.ToLower(entry.Content), lowerQuery) {
				score += boost
			}

			matches = append(matches, scoredMatch{
				chunkID: entry.ChunkID,
				score:   score,
			})
		}

		// 6. Sort by score descending
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].score > matches[j].score
		})

		// 7. Take top K and fetch details
		// We process all matches to handle deduplication correctly before pagination

		format, _ := cmd.Flags().GetString("format")
		outputPath, _ := cmd.Flags().GetString("out")
		contextSize, _ := cmd.Flags().GetInt("context")

		var output []ExportResult
		seen := make(map[string]bool)

		for _, match := range matches {
			path, content, index, err := db.GetChunkDetails(match.chunkID)
			if err != nil {
				slog.Error("failed to get chunk details", "id", match.chunkID, "error", err)
				continue
			}

			// Deduplicate: same file + same content (snippet)
			key := path + "|" + content
			if seen[key] {
				continue
			}
			seen[key] = true

			snippet := content
			if contextSize > 0 && contextSize < len(content) {
				snippet = getContextSnippet(content, query, contextSize)
			}

			// Apply offset: skip first 'offset' unique results
			if len(seen)-1 < offset {
				continue
			}

			output = append(output, ExportResult{
				Rank:    offset + len(output) + 1,
				Score:   match.score,
				Path:    path,
				ChunkID: match.chunkID,
				Index:   index,
				Snippet: snippet,
			})

			// Stop if we reached the limit *after* deduplication and offset
			if len(output) >= limit {
				break
			}
		}

		return exportResults(output, format, outputPath, query)
	},
}

func getContextSnippet(content, query string, contextSize int) string {
	// Simple approach: find the query in the content (case-insensitive)
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)
	
	// Try to find the exact query match if possible
	idx := strings.Index(lowerContent, lowerQuery)
	if idx == -1 {
		// If not found (e.g. semantic match without exact words), just take the beginning
		if len(content) <= contextSize {
			return content
		}
		return content[:contextSize] + "..."
	}

	start := idx - (contextSize / 2)
	if start < 0 {
		start = 0
	}
	end := start + contextSize
	if end > len(content) {
		end = len(content)
		start = end - contextSize
		if start < 0 { start = 0 }
	}

	prefix := ""
	if start > 0 {
		prefix = "..."
	}
	suffix := ""
	if end < len(content) {
		suffix = "..."
	}

	return prefix + strings.TrimSpace(content[start:end]) + suffix
}

type ExportResult struct {
	Rank    int     `json:"rank"`
	Score   float32 `json:"score"`
	Path    string  `json:"path"`
	ChunkID int64   `json:"chunk_id"`
	Index   int     `json:"index"`
	Snippet string  `json:"snippet"`
}

func exportResults(results []ExportResult, format, outputPath, query string) error {
	var out io.Writer = os.Stdout
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	switch format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	case "jsonl":
		enc := json.NewEncoder(out)
		for _, r := range results {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return nil
	case "csv":
		writer := csv.NewWriter(out)
		writer.Write([]string{"Rank", "Score", "Path", "ChunkID", "Index", "Snippet"})
		for _, r := range results {
			writer.Write([]string{
				fmt.Sprintf("%d", r.Rank),
				fmt.Sprintf("%.4f", r.Score),
				r.Path,
				fmt.Sprintf("%d", r.ChunkID),
				fmt.Sprintf("%d", r.Index),
				r.Snippet,
			})
		}
		writer.Flush()
		return writer.Error()
	case "table":
		table := tablewriter.NewTable(out)
		table.Header("Rank", "Score", "Path", "Snippet")
		for _, r := range results {
			table.Append(
				fmt.Sprintf("%d", r.Rank),
				fmt.Sprintf("%.4f", r.Score),
				r.Path,
				r.Snippet,
			)
		}
		table.Render()
		return nil
	case "md":
		fmt.Fprintln(out, "| Rank | Score | Path | Snippet |")
		fmt.Fprintln(out, "|------|-------|------|---------|")
		for _, r := range results {
			// Clean snippet for Markdown (no newlines)
			snippet := strings.ReplaceAll(r.Snippet, "\n", " ")
			if len(snippet) > 100 {
				snippet = snippet[:97] + "..."
			}
			fmt.Fprintf(out, "| %d | %.4f | %s | %s |\n", r.Rank, r.Score, r.Path, snippet)
		}
		return nil
	default:
		// Default Text
		fmt.Fprintf(out, "\n🔍 Search results for: %q\n", query)
		fmt.Fprintln(out, "--------------------------------------------------")
		for _, r := range results {
			fmt.Fprintf(out, "%d. [%.4f] %s (Chunk #%d)\n", r.Rank, r.Score, r.Path, r.Index)
			fmt.Fprintf(out, "   %s\n\n", r.Snippet)
		}
		if len(results) == 0 {
			fmt.Fprintln(out, "No results found.")
		}
		return nil
	}
}

func init() {
	searchCmd.Flags().StringP("db", "d", "vec.db", "path to the vector database")
	searchCmd.Flags().StringP("query", "q", "", "search query")
	searchCmd.Flags().IntP("limit", "l", 5, "max number of results")
	searchCmd.Flags().String("runtime", "", "path to libonnxruntime.so")
	searchCmd.Flags().String("model", "model.onnx", "path to the ONNX model file")
	searchCmd.Flags().String("vocab", "vocab.txt", "path to the tokenizer vocabulary file")
	searchCmd.Flags().StringP("format", "f", "text", "output format (text, json, jsonl, table, csv, md)")
	searchCmd.Flags().StringP("out", "o", "", "write results to a file instead of stdout")
	searchCmd.Flags().IntP("context", "c", 200, "max characters of context around the match")
	searchCmd.Flags().Int("offset", 0, "number of results to skip")
	searchCmd.Flags().Float32P("boost", "b", 0.5, "score boost for exact string matches")

	rootCmd.AddCommand(searchCmd)
}
