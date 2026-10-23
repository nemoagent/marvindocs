package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI embeds texts through the /embeddings endpoint of an
// OpenAI-compatible server such as vLLM.
type OpenAI struct {
	baseURL string
	model   string
	dim     int
	client  *http.Client
}

// NewOpenAI returns an Embedder for model served at baseURL, which
// includes the API version (for example http://localhost:8000/v1).
// The dimension defaults to 1024 (bge-m3).
func NewOpenAI(baseURL, model string) *OpenAI {
	return &OpenAI{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		dim:     1024,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// Embed sends texts in one request and returns their normalised vectors
// in input order.
func (o *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": o.model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("openai embed: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("openai embed: decode response: %w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("openai embed: got %d vectors for %d texts", len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(vecs) || vecs[d.Index] != nil {
			return nil, fmt.Errorf("openai embed: bad vector index %d", d.Index)
		}
		normalize(d.Embedding)
		vecs[d.Index] = d.Embedding
	}
	return vecs, nil
}

// Dim returns the vector length.
func (o *OpenAI) Dim() int { return o.dim }

// Close is a no-op.
func (o *OpenAI) Close() error { return nil }
