package auth

import (
	"strings"
	"testing"
)

func TestPassword_HashVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Fatalf("verify should succeed: %v", err)
	}
	if err := VerifyPassword("wrong", hash); err != ErrPasswordMismatch {
		t.Fatalf("wrong password should mismatch, got %v", err)
	}
}

func TestPassword_SaltIsRandom(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("hashes of the same password must differ (random salt)")
	}
}

func TestPassword_MalformedHash(t *testing.T) {
	for _, bad := range []string{"", "plain", "$argon2id$bad", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA"} {
		if err := VerifyPassword("x", bad); err != ErrPasswordMismatch {
			t.Errorf("VerifyPassword(%q) = %v, want mismatch", bad, err)
		}
	}
}
