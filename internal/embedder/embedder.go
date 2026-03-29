package embedder

// Embedder defines the interface for text embedding.
type Embedder interface {
	// Embed generates a vector embedding for a single text string.
	Embed(text string) ([]float32, error)

	// EmbedBatch generates vector embeddings for multiple texts.
	EmbedBatch(texts []string) ([][]float32, error)

	// Close releases any resources held by the embedder.
	Close() error
}
