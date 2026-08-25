package auth

import "testing"

func TestHashToken_Deterministic(t *testing.T) {
	a := HashToken("hello")
	b := HashToken("hello")
	if a != b {
		t.Fatal("same input must hash the same")
	}
	if a == HashToken("world") {
		t.Fatal("different input must hash differently")
	}
	if len(a) != 64 { // sha256 hex
		t.Fatalf("hash length = %d, want 64", len(a))
	}
}

func TestGenerateToken_UniqueAndNonEmpty(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if tok == "" {
			t.Fatal("token must not be empty")
		}
		if seen[tok] {
			t.Fatalf("duplicate token generated: %s", tok)
		}
		seen[tok] = true
	}
}
