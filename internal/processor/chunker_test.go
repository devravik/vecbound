package processor

import (
	"testing"
)

func TestChunker(t *testing.T) {
	text := "one two three four five six seven eight nine ten"
	chunker := NewChunker(4, 2)
	chunks := chunker.ChunkText(text)

	// Expected:
	// "one two three four"
	// "three four five six"
	// "five six seven eight"
	// "seven eight nine ten"
	
	if len(chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(chunks))
	}

	expectedFirst := "one two three four"
	if chunks[0].Content != expectedFirst {
		t.Errorf("expected '%s', got '%s'", expectedFirst, chunks[0].Content)
	}

	expectedSecond := "three four five six"
	if chunks[1].Content != expectedSecond {
		t.Errorf("expected '%s', got '%s'", expectedSecond, chunks[1].Content)
	}
}
