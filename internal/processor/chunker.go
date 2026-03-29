package processor

import (
	"strings"
)

// Chunk represents a piece of text with metadata.
type Chunk struct {
	Content string
	Index   int
}

// Chunker handles splitting text into overlapping windows.
type Chunker struct {
	Size    int // Number of words per chunk
	Overlap int // Number of overlapping words
}

// NewChunker creates a new Chunker instance.
func NewChunker(size, overlap int) *Chunker {
	return &Chunker{
		Size:    size,
		Overlap: overlap,
	}
}

// ChunkText splits the input string into overlapping chunks based on word count.
func (c *Chunker) ChunkText(text string) []Chunk {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []Chunk
	step := c.Size - c.Overlap
	if step <= 0 {
		step = 1 // Prevent infinite loop if overlap >= size
	}

	for i := 0; i < len(words); i += step {
		end := i + c.Size
		if end > len(words) {
			end = len(words)
		}

		content := strings.Join(words[i:end], " ")
		chunks = append(chunks, Chunk{
			Content: content,
			Index:   len(chunks),
		})

		if end == len(words) {
			break
		}
	}

	return chunks
}
