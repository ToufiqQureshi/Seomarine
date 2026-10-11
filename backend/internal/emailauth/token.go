package emailauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var (
	// ErrTokenExpired means a well-formed token is past its expiry.
	ErrTokenExpired = errors.New("token expired")
	// ErrTokenInvalid means the token is malformed, signed with another key or
	// algorithm, or carries no expiry.
	ErrTokenInvalid = errors.New("invalid token")
)

var b64 = base64.RawURLEncoding

// jwtHeader is the only header Better Auth issues and the only one accepted.
const jwtHeader = `{"alg":"HS256"}`

// signJWT signs claims with HS256 and adds iat and exp, like Better Auth's
// email-verification tokens.
func signJWT(secret []byte, claims map[string]any, expiresIn time.Duration, now time.Time) (string, error) {
	payload := make(map[string]any, len(claims)+2)
	for k, v := range claims {
		payload[k] = v
	}
	payload["iat"] = now.Unix()
	payload["exp"] = now.Add(expiresIn).Unix()
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode token claims: %w", err)
	}
	signingInput := b64.EncodeToString([]byte(jwtHeader)) + "." + b64.EncodeToString(body)
	return signingInput + "." + b64.EncodeToString(jwtSignature(secret, signingInput)), nil
}

func jwtSignature(secret []byte, signingInput string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return mac.Sum(nil)
}

// verifyJWT checks the signature, algorithm and expiry and returns the claims.
// Only HS256 is accepted, so a token cannot pick a weaker algorithm for itself.
func verifyJWT(secret []byte, token string, now time.Time) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	headerBytes, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Alg != "HS256" {
		return nil, ErrTokenInvalid
	}
	signature, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if !hmac.Equal(signature, jwtSignature(secret, parts[0]+"."+parts[1])) {
		return nil, ErrTokenInvalid
	}
	body, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var claims map[string]any
	if json.Unmarshal(body, &claims) != nil {
		return nil, ErrTokenInvalid
	}
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, ErrTokenInvalid
	}
	if !now.Before(time.Unix(int64(exp), 0)) {
		return nil, ErrTokenExpired
	}
	return claims, nil
}

const idAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// newID returns n random letters and digits, the id format Better Auth uses for
// rows and tokens.
func newID(n int) (string, error) {
	out := make([]byte, n)
	limit := big.NewInt(int64(len(idAlphabet)))
	for i := range out {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate id: %w", err)
		}
		out[i] = idAlphabet[index.Int64()]
	}
	return string(out), nil
}
