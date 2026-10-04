package embed

import (
	"math"
	"math/rand"
	"testing"
)

func randomUnit(r *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(r.NormFloat64())
	}
	normalize(v)
	return v
}

func TestQuantizeRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	v := randomUnit(r, 1024)
	blob := Quantize(v)
	if len(blob) != 4+len(v) {
		t.Fatalf("blob len = %d, want %d", len(blob), 4+len(v))
	}
	scale := math.Float32frombits(uint32(blob[0]) | uint32(blob[1])<<8 | uint32(blob[2])<<16 | uint32(blob[3])<<24)
	for i, x := range v {
		got := scale * float32(int8(blob[4+i]))
		if d := math.Abs(float64(got - x)); d >= 1e-2 {
			t.Fatalf("element %d: got %v, want %v", i, got, x)
		}
	}
}

func TestDot(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	v := randomUnit(r, 1024)
	if d := Dot(v, Quantize(v)); math.Abs(float64(d)-1) > 1e-2 {
		t.Fatalf("Dot(v, Quantize(v)) = %v, want ~1", d)
	}
	if d := Dot(v[:10], Quantize(v)); d != 0 {
		t.Fatalf("Dot with mismatched length = %v, want 0", d)
	}
	zero := make([]float32, 8)
	if d := Dot(v[:8], Quantize(zero)); d != 0 {
		t.Fatalf("Dot with zero vector = %v, want 0", d)
	}
}

func BenchmarkDot(b *testing.B) {
	r := rand.New(rand.NewSource(3))
	q := randomUnit(r, 1024)
	blob := Quantize(randomUnit(r, 1024))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Dot(q, blob)
	}
}
