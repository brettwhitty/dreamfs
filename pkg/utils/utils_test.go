package utils

import (
	"strings"
	"testing"
)

// TestGenerateUUID_Deterministic verifies that GenerateUUID (v5/SHA1) produces
// the same output for the same input every time.
func TestGenerateUUID_Deterministic(t *testing.T) {
	input := "host-abc|PHYS:SN12345|/data/file.txt|2026-01-01T00:00:00Z|1000|aabbcc"
	u1 := GenerateUUID(input)
	u2 := GenerateUUID(input)
	if u1 != u2 {
		t.Fatalf("GenerateUUID is not deterministic: %q vs %q", u1, u2)
	}
	if u1 == "" {
		t.Fatal("GenerateUUID returned empty string")
	}
}

// TestGenerateUUID_Unique verifies that different inputs yield different UUIDs.
func TestGenerateUUID_Unique(t *testing.T) {
	u1 := GenerateUUID("input-one")
	u2 := GenerateUUID("input-two")
	if u1 == u2 {
		t.Fatalf("GenerateUUID produced identical output for different inputs: %q", u1)
	}
}

// TestGenerateUUID_Format verifies the output is formatted as a standard UUID string.
func TestGenerateUUID_Format(t *testing.T) {
	u := GenerateUUID("any-string")
	parts := strings.Split(u, "-")
	if len(parts) != 5 {
		t.Fatalf("UUID %q does not have 5 hyphen-separated groups", u)
	}
}

// TestShortenString_Roundtrip verifies base64 encoding is stable.
func TestShortenString_NonEmpty(t *testing.T) {
	s := ShortenString("test data for shortening")
	if s == "" {
		t.Fatal("ShortenString returned empty string")
	}
	// URL-safe base64 should not contain +, /, or = padding.
	for _, ch := range []string{"+", "/", "="} {
		if strings.Contains(s, ch) {
			t.Fatalf("ShortenString output %q contains non-URL-safe char %q", s, ch)
		}
	}
}
