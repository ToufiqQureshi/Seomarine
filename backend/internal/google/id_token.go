package google

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IDTokenVerifier validates Google's signed identity token before a grant is saved.
type IDTokenVerifier struct {
	Client   *http.Client
	KeysURL  string
	Now      func() time.Time
	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	keysTill time.Time
}

type googleJWKSet struct {
	Keys []struct {
		KID string `json:"kid"`
		KTY string `json:"kty"`
		ALG string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *IDTokenVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v *IDTokenVerifier) keyURL() string {
	if v.KeysURL != "" {
		return v.KeysURL
	}
	return "https://www.googleapis.com/oauth2/v3/certs"
}

func (v *IDTokenVerifier) client() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return &http.Client{Timeout: 8 * time.Second}
}

func (v *IDTokenVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.now().Before(v.keysTill) {
		if key := v.keys[kid]; key != nil {
			return key, nil
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, v.keyURL(), nil)
	if err != nil {
		return nil, errors.New("google identity keys unavailable")
	}
	resp, err := v.client().Do(req)
	if err != nil {
		return nil, errors.New("google identity keys unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("google identity keys unavailable")
	}
	var set googleJWKSet
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&set); err != nil {
		return nil, errors.New("invalid google identity keys")
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, candidate := range set.Keys {
		if candidate.KID == "" || candidate.KTY != "RSA" || candidate.ALG != "RS256" || candidate.Use != "sig" {
			continue
		}
		nBytes, nErr := base64.RawURLEncoding.DecodeString(candidate.N)
		eBytes, eErr := base64.RawURLEncoding.DecodeString(candidate.E)
		if nErr != nil || eErr != nil || len(nBytes) < 256 || len(eBytes) == 0 || len(eBytes) > 4 {
			continue
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		if e < 3 || e%2 == 0 {
			continue
		}
		keys[candidate.KID] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}
	v.keys = keys
	v.keysTill = v.now().Add(5 * time.Minute)
	if key := keys[kid]; key != nil {
		return key, nil
	}
	return nil, errors.New("google identity key unavailable")
}

// AccountID verifies an RS256 Google ID token and returns its stable subject.
func (v *IDTokenVerifier) AccountID(ctx context.Context, token, clientID string) (string, error) {
	if v == nil || token == "" || clientID == "" || len(token) > 16<<10 {
		return "", errors.New("invalid google identity token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid google identity token")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(headerBytes) > 2048 {
		return "", errors.New("invalid google identity token")
	}
	var header struct {
		ALG string `json:"alg"`
		KID string `json:"kid"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.ALG != "RS256" || header.KID == "" {
		return "", errors.New("invalid google identity token")
	}
	key, err := v.key(ctx, header.KID)
	if err != nil {
		return "", err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("invalid google identity token")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return "", errors.New("invalid google identity signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payloadBytes) > 8<<10 {
		return "", errors.New("invalid google identity claims")
	}
	var claims struct {
		Issuer   string `json:"iss"`
		Audience string `json:"aud"`
		Subject  string `json:"sub"`
		Expiry   int64  `json:"exp"`
		IssuedAt int64  `json:"iat"`
	}
	if json.Unmarshal(payloadBytes, &claims) != nil || claims.Audience != clientID || claims.Subject == "" || len(claims.Subject) > 256 ||
		(claims.Issuer != "https://accounts.google.com" && claims.Issuer != "accounts.google.com") ||
		claims.Expiry <= v.now().Unix() || claims.IssuedAt > v.now().Add(time.Minute).Unix() || claims.IssuedAt <= 0 {
		return "", errors.New("invalid google identity claims")
	}
	return claims.Subject, nil
}
