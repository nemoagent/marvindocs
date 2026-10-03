package main

import "testing"

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:3333":     true,
		"localhost:3333":     true,
		"[::1]:3333":         true,
		"127.5.5.5:80":       true,
		"0.0.0.0:3333":       false,
		"192.168.1.5:3333":   false,
		"[::]:3333":          false,
		"example.local:3333": false,
		"3333":               false,
	}
	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestNormalizeListenAddr(t *testing.T) {
	cases := map[string]string{
		"":             "",
		":3333":        "127.0.0.1:3333",
		"0.0.0.0:3333": "0.0.0.0:3333",
	}
	for in, want := range cases {
		if got := normalizeListenAddr(in); got != want {
			t.Errorf("normalizeListenAddr(%q) = %q, want %q", in, got, want)
		}
	}
}
