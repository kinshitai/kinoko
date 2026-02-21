//go:build embedding

package embedding

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sync"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	hiddenSize = 384
	modelName  = "bge-small-en-v1.5"
)

// ONNXEngine implements Engine using ONNX Runtime with a BGE-small model.
//
// Thread-safe: the tokenizer and ONNX session are protected by a mutex.
// ONNX Runtime sessions can be shared across goroutines, but we serialize
// access to avoid tensor reuse issues.
type ONNXEngine struct {
	tokenizer *tokenizers.Tokenizer
	modelPath string
	mu        sync.Mutex
	closed    bool
}

// ortInitOnce ensures ONNX Runtime environment is initialized exactly once per process.
var ortInitOnce sync.Once
var ortInitErr error

// NewONNXEngine loads the ONNX model and tokenizer from modelDir.
// modelDir must contain model.onnx, tokenizer.json, and libonnxruntime.so.
func NewONNXEngine(modelDir string) (*ONNXEngine, error) {
	ortLib := filepath.Join(modelDir, "libonnxruntime.so")
	modelPath := filepath.Join(modelDir, "model.onnx")
	tokenizerPath := filepath.Join(modelDir, "tokenizer.json")

	ort.SetSharedLibraryPath(ortLib)
	ortInitOnce.Do(func() {
		ortInitErr = ort.InitializeEnvironment()
	})
	if ortInitErr != nil {
		return nil, fmt.Errorf("init onnx runtime: %w", ortInitErr)
	}

	tk, err := tokenizers.FromFile(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	return &ONNXEngine{
		tokenizer: tk,
		modelPath: modelPath,
	}, nil
}

func (e *ONNXEngine) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (e *ONNXEngine) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, fmt.Errorf("engine closed")
	}

	// Check context before doing work.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	results := make([][]float32, len(texts))
	// Process one at a time — simpler tensor management, and BGE-small inference
	// is fast enough (~1ms per sentence) that batching overhead isn't worth it yet.
	for i, text := range texts {
		vec, err := e.embedSingle(text)
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		results[i] = vec

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return results, nil
}

func (e *ONNXEngine) embedSingle(text string) ([]float32, error) {
	ids, _ := e.tokenizer.Encode(text, true) // returns ([]uint32 ids, []string tokens)
	seqLen := int64(len(ids))
	batchSize := int64(1)

	inputIDs := make([]int64, seqLen)
	attnMask := make([]int64, seqLen)
	tokenTypeIDs := make([]int64, seqLen)
	for i, id := range ids {
		inputIDs[i] = int64(id)
		attnMask[i] = 1
	}

	shape := ort.Shape{batchSize, seqLen}

	inputIDsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("create input_ids tensor: %w", err)
	}
	defer inputIDsTensor.Destroy()

	attnMaskTensor, err := ort.NewTensor(shape, attnMask)
	if err != nil {
		return nil, fmt.Errorf("create attention_mask tensor: %w", err)
	}
	defer attnMaskTensor.Destroy()

	tokenTypeTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("create token_type_ids tensor: %w", err)
	}
	defer tokenTypeTensor.Destroy()

	outputShape := ort.Shape{batchSize, seqLen, hiddenSize}
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outputTensor.Destroy()

	session, err := ort.NewAdvancedSession(e.modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.ArbitraryTensor{inputIDsTensor, attnMaskTensor, tokenTypeTensor},
		[]ort.ArbitraryTensor{outputTensor},
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer session.Destroy()

	if err := session.Run(); err != nil {
		return nil, fmt.Errorf("run inference: %w", err)
	}

	// CLS pooling: take first token's hidden state.
	output := outputTensor.GetData()
	embedding := make([]float32, hiddenSize)
	copy(embedding, output[:hiddenSize])

	// L2 normalize.
	var norm float64
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for j := range embedding {
			embedding[j] = float32(float64(embedding[j]) / norm)
		}
	}

	return embedding, nil
}

func (e *ONNXEngine) Dims() int       { return hiddenSize }
func (e *ONNXEngine) ModelID() string { return modelName }

func (e *ONNXEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	e.tokenizer.Close()
	return nil
}
