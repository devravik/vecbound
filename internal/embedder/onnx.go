package embedder

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	ort "github.com/yalue/onnxruntime_go"
)

// ONNXEmbedder wraps direct ONNX Runtime inference for all-MiniLM-L6-v2.
type ONNXEmbedder struct {
	session *ort.DynamicAdvancedSession
	vocab   map[string]int64
	maxLen  int
}

// NewONNXEmbedder creates a new embedder.
// runtimePath: path to libonnxruntime.so (optional)
// modelPath: path to the all-MiniLM-L6-v2 ONNX model file
// vocabPath: path to the vocab.txt file
func NewONNXEmbedder(runtimePath, modelPath, vocabPath string) (*ONNXEmbedder, error) {
	// 1. Smart Discovery for Runtime
	if runtimePath == "" {
		cwd, _ := os.Getwd()
		execPath, _ := os.Executable()
		execDir := filepath.Dir(execPath)

		candidates := []string{
			filepath.Join(cwd, "libonnxruntime.so"),
			filepath.Join(execDir, "libonnxruntime.so"),
			"libonnxruntime.so", // Fallback to system path
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				if abs, err := filepath.Abs(c); err == nil {
					runtimePath = abs
					break
				}
			}
		}
	}

	if runtimePath != "" {
		ort.SetSharedLibraryPath(runtimePath)
	}

	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("failed to init ONNX environment: %w", err)
	}

	// 2. Smart Discovery for Model and Vocab
	modelPath = discoverFile(modelPath, "model.onnx")
	vocabPath = discoverFile(vocabPath, "vocab.txt")

	// Load vocabulary
	vocab, err := loadVocab(vocabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load vocab from %s: %w", vocabPath, err)
	}

	// Create session with dynamic inputs/outputs
	inputNames := []string{"input_ids", "attention_mask", "token_type_ids"}
	outputNames := []string{"last_hidden_state"}

	session, err := ort.NewDynamicAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create ONNX session: %w", err)
	}

	return &ONNXEmbedder{
		session: session,
		vocab:   vocab,
		maxLen:  128,
	}, nil
}

// Embed generates an embedding for a single text string.
// Returns a 384-dimensional vector (all-MiniLM-L6-v2 output).
func (e *ONNXEmbedder) Embed(text string) ([]float32, error) {
	// Tokenize
	tokens := e.tokenize(text)

	// Create input tensors
	batchSize := int64(1)
	seqLen := int64(len(tokens))
	shape := ort.NewShape(batchSize, seqLen)

	inputIDs := make([]int64, seqLen)
	attentionMask := make([]int64, seqLen)
	tokenTypeIDs := make([]int64, seqLen)

	for i, t := range tokens {
		inputIDs[i] = t
		attentionMask[i] = 1
		tokenTypeIDs[i] = 0
	}

	inputIDsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attentionMaskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer attentionMaskTensor.Destroy()

	tokenTypeIDsTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
	}
	defer tokenTypeIDsTensor.Destroy()

	// Prepare inputs and outputs for Run()
	inputs := []ort.Value{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor}
	outputs := []ort.Value{nil} // Let ONNX Runtime allocate the output

	// Run inference
	err = e.session.Run(inputs, outputs)
	if err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				o.Destroy()
			}
		}
	}()

	// Extract output: shape [1, seq_len, 384]
	outputTensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output tensor type")
	}

	hiddenStates := outputTensor.GetData()
	hiddenDim := 384

	// Mean pooling over the sequence dimension
	embedding := make([]float32, hiddenDim)
	validTokens := int(seqLen)
	for i := 0; i < validTokens; i++ {
		for j := 0; j < hiddenDim; j++ {
			embedding[j] += hiddenStates[i*hiddenDim+j]
		}
	}
	for j := 0; j < hiddenDim; j++ {
		embedding[j] /= float32(validTokens)
	}

	// L2 normalize
	normalize(embedding)

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts.
func (e *ONNXEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		result, err := e.Embed(text)
		if err != nil {
			return nil, fmt.Errorf("batch embed failed at index %d: %w", i, err)
		}
		embeddings[i] = result
	}
	return embeddings, nil
}

// Close releases resources.
func (e *ONNXEmbedder) Close() error {
	if e.session != nil {
		e.session.Destroy()
	}
	return ort.DestroyEnvironment()
}

// tokenize performs basic WordPiece-style tokenization.
func (e *ONNXEmbedder) tokenize(text string) []int64 {
	clsID := e.lookupToken("[CLS]")
	sepID := e.lookupToken("[SEP]")
	unkID := e.lookupToken("[UNK]")

	tokens := []int64{clsID}

	words := splitWords(text)
	for _, word := range words {
		wordTokens := e.wordPieceTokenize(word, unkID)
		if len(tokens)+len(wordTokens)+1 > e.maxLen {
			break
		}
		tokens = append(tokens, wordTokens...)
	}

	tokens = append(tokens, sepID)
	return tokens
}

// wordPieceTokenize splits a single word into WordPiece sub-tokens.
func (e *ONNXEmbedder) wordPieceTokenize(word string, unkID int64) []int64 {
	word = strings.ToLower(word)
	var tokens []int64

	start := 0
	for start < len(word) {
		end := len(word)
		found := false
		for end > start {
			substr := word[start:end]
			if start > 0 {
				substr = "##" + substr
			}
			if id, ok := e.vocab[substr]; ok {
				tokens = append(tokens, id)
				found = true
				start = end
				break
			}
			end--
		}
		if !found {
			tokens = append(tokens, unkID)
			start++
		}
	}

	return tokens
}

func (e *ONNXEmbedder) lookupToken(token string) int64 {
	if id, ok := e.vocab[token]; ok {
		return id
	}
	return 0
}

func loadVocab(path string) (map[string]int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try JSON format first: {"token": id, ...}
	vocab := make(map[string]int64)
	if err := json.Unmarshal(data, &vocab); err == nil {
		return vocab, nil
	}

	// Fall back to line-based vocab.txt format (one token per line)
	vocab = make(map[string]int64)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			vocab[line] = int64(i)
		}
	}
	return vocab, nil
}

func splitWords(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			if unicode.IsPunct(r) {
				words = append(words, string(r))
			}
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sum))
	if norm > 0 {
		for i := range v {
			v[i] /= norm
		}
	}
}

func discoverFile(path, defaultName string) string {
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	candidates := []string{
		defaultName,
		"./" + defaultName,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return path // Fallback to original
}
