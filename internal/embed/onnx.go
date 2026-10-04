//go:build onnx

package embed

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"
)

// maxTokens is the longest input fed to the model; longer texts are cut.
const maxTokens = 512

var ortOnce sync.Once
var ortErr error

// ONNX embeds texts locally with a bge-m3 int8 ONNX model (CLS pooling).
type ONNX struct {
	mu   sync.Mutex
	tk   *tokenizers.Tokenizer
	sess *ort.DynamicAdvancedSession
	in   []string
	dim  int
}

// NewONNX loads model_int8.onnx and tokenizer.json from modelDir and
// the ONNX Runtime shared library at libPath.
func NewONNX(modelDir, libPath string) (*ONNX, error) {
	ortOnce.Do(func() {
		ort.SetSharedLibraryPath(libPath)
		ortErr = ort.InitializeEnvironment()
	})
	if ortErr != nil {
		return nil, fmt.Errorf("init onnxruntime: %w", ortErr)
	}
	tk, err := tokenizers.FromFile(filepath.Join(modelDir, "tokenizer.json"))
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	modelPath := filepath.Join(modelDir, "model_int8.onnx")
	ins, outs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		_ = tk.Close()
		return nil, fmt.Errorf("read model info: %w", err)
	}
	if len(outs) == 0 {
		_ = tk.Close()
		return nil, fmt.Errorf("model %s has no outputs", modelPath)
	}
	inNames := make([]string, len(ins))
	for i, in := range ins {
		inNames[i] = in.Name
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		_ = tk.Close()
		return nil, err
	}
	defer func() { _ = opts.Destroy() }()
	sess, err := ort.NewDynamicAdvancedSession(modelPath, inNames, []string{outs[0].Name}, opts)
	if err != nil {
		_ = tk.Close()
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &ONNX{tk: tk, sess: sess, in: inNames, dim: 1024}, nil
}

// Embed encodes texts one by one and returns their normalised CLS vectors.
func (o *ONNX) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v, err := o.embedOne(text)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (o *ONNX) embedOne(text string) ([]float32, error) {
	e := o.tk.EncodeWithOptions(text, true, tokenizers.WithReturnAttentionMask())
	ids, mask := e.IDs, e.AttentionMask
	if len(ids) > maxTokens {
		ids, mask = ids[:maxTokens], mask[:maxTokens]
	}
	n := int64(len(ids))
	idData := make([]int64, n)
	maskData := make([]int64, n)
	for i := range ids {
		idData[i] = int64(ids[i])
		maskData[i] = int64(mask[i])
	}
	idT, err := ort.NewTensor(ort.NewShape(1, n), idData)
	if err != nil {
		return nil, err
	}
	defer func() { _ = idT.Destroy() }()
	maskT, err := ort.NewTensor(ort.NewShape(1, n), maskData)
	if err != nil {
		return nil, err
	}
	defer func() { _ = maskT.Destroy() }()

	// Inputs follow the model's own order: input_ids, attention_mask.
	inputs := make([]ort.Value, len(o.in))
	for i, name := range o.in {
		switch name {
		case "input_ids":
			inputs[i] = idT
		case "attention_mask":
			inputs[i] = maskT
		default:
			return nil, fmt.Errorf("unsupported model input %q", name)
		}
	}
	outputs := []ort.Value{nil}
	if err := o.sess.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("onnx run: %w", err)
	}
	defer func() { _ = outputs[0].Destroy() }()
	ot, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output type %T", outputs[0])
	}
	data := ot.GetData()
	if len(data) < o.dim {
		return nil, fmt.Errorf("output has %d values, want at least %d", len(data), o.dim)
	}
	v := make([]float32, o.dim)
	copy(v, data[:o.dim]) // CLS token is the first row
	normalize(v)
	return v, nil
}

// Dim returns the vector length.
func (o *ONNX) Dim() int { return o.dim }

// Close releases the session and the tokenizer.
func (o *ONNX) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	err := o.sess.Destroy()
	if cerr := o.tk.Close(); err == nil {
		err = cerr
	}
	return err
}
