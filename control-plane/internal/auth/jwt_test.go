package auth

import (
	"testing"
	"time"
)

func TestJWT_RoundTrip(t *testing.T) {
	secret := "test-secret"
	tok, err := IssueJWT(secret, "user-123", time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	c, err := ParseJWT(secret, tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Subject != "user-123" {
		t.Fatalf("subject = %q, want user-123", c.Subject)
	}
	if !c.ExpiresAt.After(time.Now()) {
		t.Fatal("expiry should be in the future")
	}
}

func TestJWT_Expired(t *testing.T) {
	tok, err := IssueJWT("s", "u", -time.Minute) // already expired
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := ParseJWT("s", tok); err != ErrTokenExpired {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
}

func TestJWT_WrongSecret(t *testing.T) {
	tok, _ := IssueJWT("right", "u", time.Hour)
	if _, err := ParseJWT("wrong", tok); err != ErrTokenInvalid {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
}

func TestJWT_Tampered(t *testing.T) {
	tok, _ := IssueJWT("s", "u", time.Hour)
	// Flip the last character of the signature.
	b := []byte(tok)
	if b[len(b)-1] == 'A' {
		b[len(b)-1] = 'B'
	} else {
		b[len(b)-1] = 'A'
	}
	if _, err := ParseJWT("s", string(b)); err != ErrTokenInvalid {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
}

func TestJWT_Malformed(t *testing.T) {
	for _, bad := range []string{"", "a.b", "only-one-part", "a.b.c.d"} {
		if _, err := ParseJWT("s", bad); err != ErrTokenInvalid {
			t.Errorf("ParseJWT(%q) err = %v, want ErrTokenInvalid", bad, err)
		}
	}
}
