//go:build onnx

package embed

import (
	"context"
	"math"
	"os"
	"testing"
)

func TestONNXEmbed(t *testing.T) {
	dir, lib := os.Getenv("MARVINDOCS_ONNX_MODEL_DIR"), os.Getenv("MARVINDOCS_ONNXRUNTIME")
	if dir == "" || lib == "" {
		t.Skip("MARVINDOCS_ONNX_MODEL_DIR and MARVINDOCS_ONNXRUNTIME not set")
	}
	e, err := NewONNX(dir, lib)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()
	vecs, err := e.Embed(context.Background(), []string{"ansible loop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 1 || len(vecs[0]) != 1024 || e.Dim() != 1024 {
		t.Fatalf("unexpected result shape: %d vectors", len(vecs))
	}
	var sum float64
	for _, x := range vecs[0] {
		sum += float64(x) * float64(x)
	}
	if math.Abs(math.Sqrt(sum)-1) > 1e-3 {
		t.Fatalf("norm = %v, want 1", math.Sqrt(sum))
	}
}
