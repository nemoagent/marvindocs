package embed

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.URL.Path != "/api/embed" || json.NewDecoder(r.Body).Decode(&req) != nil || req.Model != "m" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out := make([][]float32, len(req.Input))
		for i := range out {
			out[i] = []float32{3, 4}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer srv.Close()

	e := NewOllama(srv.URL+"/", "m")
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || math.Abs(float64(vecs[0][0])-0.6) > 1e-6 || math.Abs(float64(vecs[0][1])-0.8) > 1e-6 {
		t.Fatalf("vecs = %v, want normalised [0.6 0.8]", vecs)
	}

	bad := NewOllama(srv.URL, "other")
	if _, err := bad.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected an error for HTTP 400")
	}
}
