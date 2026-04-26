package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipPool reuses gzip.Writer instances across requests. Encoding a
// response from scratch costs a few KB of state allocation; with a
// pool the steady-state cost is one Reset per response.
var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

// gzipResponseWriter wraps http.ResponseWriter and transparently
// encodes the body. It defers WriteHeader until the first Write so
// that handlers can still set/modify headers (Content-Type, ETag,
// status code) before bytes are flushed to the gzip stream.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true
	h := g.ResponseWriter.Header()
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	// Gzip changes the body length; the original Content-Length is wrong.
	h.Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(p []byte) (int, error) {
	if !g.wroteHeader {
		if g.ResponseWriter.Header().Get("Content-Type") == "" {
			g.ResponseWriter.Header().Set("Content-Type", http.DetectContentType(p))
		}
		g.WriteHeader(http.StatusOK)
	}
	return g.gz.Write(p)
}

// gzipHandler wraps h to gzip-encode responses for clients that
// advertise Accept-Encoding: gzip. Skipped automatically for clients
// that don't support it. Body sizes under ~1 KB still get compressed —
// the overhead is negligible and most doc fragments compress 5-10x.
func gzipHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			h.ServeHTTP(w, r)
			return
		}
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			_ = gz.Close()
			gzipPool.Put(gz)
		}()
		grw := &gzipResponseWriter{ResponseWriter: w, gz: gz}
		h.ServeHTTP(grw, r)
	})
}
