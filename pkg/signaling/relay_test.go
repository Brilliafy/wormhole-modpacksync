package signaling

import (
	"strings"
	"testing"
)

func TestGenerateRoomCodeFormat(t *testing.T) {
	code := GenerateRoomCode()
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("expected XXXX-XXXX format, got %q", code)
	}
	for _, r := range strings.ReplaceAll(code, "-", "") {
		if !strings.ContainsRune(roomCodeAlphabet, r) {
			t.Fatalf("code %q contains character outside alphabet: %q", code, r)
		}
	}
}

func TestGenerateRoomCodeUnique(t *testing.T) {
	const n = 50000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		code := GenerateRoomCode()
		if _, dup := seen[code]; dup {
			t.Fatalf("duplicate code generated: %q", code)
		}
		seen[code] = struct{}{}
	}
}

func TestRelayURLFromEnv(t *testing.T) {
	t.Setenv("WORMHOLE_RELAY", "")
	if got := RelayURLFromEnv(); got != "http://127.0.0.1:42425" {
		t.Fatalf("expected localhost fallback, got %q", got)
	}

	t.Setenv("WORMHOLE_RELAY", "https://relay.example.org/, http://second:1234")
	if got := RelayURLFromEnv(); got != "https://relay.example.org" {
		t.Fatalf("expected first relay with trailing slash trimmed, got %q", got)
	}
}
