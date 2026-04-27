// Package logging configures the global slog handler used across
// marvindocs. Centralizing it here keeps cmd/marvindocs from juggling
// flags and JSON-vs-text decisions.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Setup installs a slog default handler for the process. The text vs
// JSON choice and the level come from environment variables so the
// behaviour is the same whether marvindocs is launched by a shell or
// by a desktop MCP client (which has no flag plumbing).
//
//	MARVINDOCS_LOG_LEVEL=debug|info|warn|error  (default: info)
//	MARVINDOCS_LOG_FORMAT=text|json              (default: text)
//
// All logs go to stderr so stdout stays clean for the JSON-RPC stdio
// transport.
func Setup() {
	setup(os.Stderr,
		os.Getenv("MARVINDOCS_LOG_LEVEL"),
		os.Getenv("MARVINDOCS_LOG_FORMAT"))
}

// setup is the testable core; Setup wires it to the real environment.
func setup(w io.Writer, levelEnv, formatEnv string) {
	level := parseLevel(levelEnv)
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if strings.EqualFold(formatEnv, "json") {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	slog.SetDefault(slog.New(h))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
