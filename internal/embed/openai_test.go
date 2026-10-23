package embed

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if r.URL.Path != "/v1/embeddings" || json.NewDecoder(r.Body).Decode(&req) != nil || req.Model != "m" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// Return the vectors in reverse order to check that index is honoured.
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		for i := len(req.Input) - 1; i >= 0; i-- {
			data = append(data, item{Index: i, Embedding: []float32{3, 4 * float32(i+1)}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()

	e := NewOpenAI(srv.URL+"/v1/", "m")
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || math.Abs(float64(vecs[0][0])-0.6) > 1e-6 || math.Abs(float64(vecs[0][1])-0.8) > 1e-6 {
		t.Fatalf("vecs[0] = %v, want normalised [0.6 0.8]", vecs)
	}
	if math.Abs(float64(vecs[1][1])-8/math.Sqrt(73)) > 1e-6 {
		t.Fatalf("vecs[1] = %v, want the vector with index 1", vecs[1])
	}

	bad := NewOpenAI(srv.URL+"/v1", "other")
	if _, err := bad.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("expected an error for HTTP 400")
	}
}
