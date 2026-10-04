//go:build !onnx

package embed

import (
	"context"
	"errors"
)

// ONNX is unavailable in this build; compile with -tags onnx.
type ONNX struct{}

// NewONNX always fails: the binary was built without the onnx tag.
func NewONNX(modelDir, libPath string) (*ONNX, error) {
	return nil, errors.New("built without onnx support (rebuild with -tags onnx)")
}

// Embed is not available without the onnx tag.
func (*ONNX) Embed(context.Context, []string) ([][]float32, error) {
	return nil, errors.New("built without onnx support")
}

// Dim returns the vector length.
func (*ONNX) Dim() int { return 1024 }

// Close is a no-op.
func (*ONNX) Close() error { return nil }
