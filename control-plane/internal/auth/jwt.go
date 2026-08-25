package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// A minimal, self-contained HS256 JWT. We implement only what the control plane
// needs (sign + verify + exp) rather than pulling in a JWT library, so the whole
// token path is auditable in one file. The `alg` is pinned to HS256 on verify to
// prevent algorithm-confusion attacks.

var (
	// ErrTokenInvalid means the token is malformed, has the wrong algorithm, or
	// failed signature verification.
	ErrTokenInvalid = errors.New("invalid token")
	// ErrTokenExpired means the signature was valid but the token has expired.
	ErrTokenExpired = errors.New("token expired")
)

// headerHS256 is the fixed, pre-encoded JOSE header {"alg":"HS256","typ":"JWT"}.
var headerHS256 = b64(`{"alg":"HS256","typ":"JWT"}`)

// Claims are the subset of registered claims we use.
type Claims struct {
	Subject   string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// IssueJWT signs a token for subject valid for ttl.
func IssueJWT(secret, subject string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("empty signing secret")
	}
	now := time.Now()
	payload, err := json.Marshal(map[string]any{
		"sub": subject,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signingInput := headerHS256 + "." + b64(string(payload))
	return signingInput + "." + sign(secret, signingInput), nil
}

// ParseJWT verifies a token's signature and expiry and returns its claims.
func ParseJWT(secret, token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrTokenInvalid
	}
	signingInput := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(secret, signingInput)), []byte(parts[2])) {
		return Claims{}, ErrTokenInvalid
	}

	// Pin the algorithm: reject "none" and any non-HS256 header.
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	var hdr struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil || hdr.Alg != "HS256" {
		return Claims{}, ErrTokenInvalid
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrTokenInvalid
	}
	var p struct {
		Sub string `json:"sub"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &p); err != nil {
		return Claims{}, ErrTokenInvalid
	}
	c := Claims{
		Subject:   p.Sub,
		IssuedAt:  time.Unix(p.Iat, 0),
		ExpiresAt: time.Unix(p.Exp, 0),
	}
	if p.Exp > 0 && time.Now().Unix() >= p.Exp {
		return c, ErrTokenExpired
	}
	return c, nil
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

func sign(secret, msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
