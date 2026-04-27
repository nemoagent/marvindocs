package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestSetupHonorsLevel(t *testing.T) {
	var buf bytes.Buffer
	setup(&buf, "warn", "text")

	slog.Info("info-msg")
	slog.Warn("warn-msg")
	slog.Error("error-msg")

	out := buf.String()
	if strings.Contains(out, "info-msg") {
		t.Errorf("info should be filtered at warn level: %s", out)
	}
	if !strings.Contains(out, "warn-msg") || !strings.Contains(out, "error-msg") {
		t.Errorf("warn/error must pass through: %s", out)
	}
}

func TestSetupJSON(t *testing.T) {
	var buf bytes.Buffer
	setup(&buf, "debug", "json")

	slog.Info("hello")
	out := buf.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Errorf("json handler not active: %s", out)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"":        slog.LevelInfo,
		"info":    slog.LevelInfo,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"garbage": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}
