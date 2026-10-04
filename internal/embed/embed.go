// Package embed turns texts into dense vectors for semantic search and
// provides the int8 quantisation used to store them in the index.
package embed

import (
	"context"
	"encoding/binary"
	"math"
)

// Embedder turns texts into L2-normalised float32 vectors of equal length.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dim() int
	Close() error
}

// Quantize packs v as symmetric int8: the first 4 bytes hold the scale
// (maxAbs/127, float32 little-endian), followed by one int8 per element.
func Quantize(v []float32) []byte {
	var maxAbs float32
	for _, x := range v {
		if a := float32(math.Abs(float64(x))); a > maxAbs {
			maxAbs = a
		}
	}
	scale := maxAbs / 127
	out := make([]byte, 4+len(v))
	binary.LittleEndian.PutUint32(out, math.Float32bits(scale))
	if scale == 0 {
		return out
	}
	for i, x := range v {
		out[4+i] = byte(int8(math.Round(float64(x / scale))))
	}
	return out
}

// Dot returns the dot product of q with a vector packed by Quantize.
// It returns 0 if the lengths do not match.
func Dot(q []float32, blob []byte) float32 {
	if len(blob) != 4+len(q) {
		return 0
	}
	scale := math.Float32frombits(binary.LittleEndian.Uint32(blob))
	data := blob[4:]
	var sum float32
	for i, x := range q {
		sum += x * float32(int8(data[i]))
	}
	return scale * sum
}

// normalize scales v to unit L2 length in place.
func normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	inv := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}
