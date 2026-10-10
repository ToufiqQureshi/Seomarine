// Package crawleraccess stores encrypted Shopify crawler credentials and
// resolves them for the Go audit crawler.
package crawleraccess

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/ids"
	"golang.org/x/crypto/chacha20poly1305"
)

const keyDirectory = "https://shopify.com/.well-known/http-message-signatures-directory"

var (
	ErrInvalidCredential = errors.New("invalid Shopify crawler signature")
	ErrWrongDomain       = errors.New("Shopify crawler signature belongs to another domain")
	ErrNotFound          = errors.New("crawler credential not found")
	signatureParams      = regexp.MustCompile(`^\w+=(\(.+)$`)
	keyIDPattern         = regexp.MustCompile(`;keyid="([^"]+)"`)
	signaturePattern     = regexp.MustCompile(`^\w+=:([A-Za-z0-9+/]+={0,2}):$`)
	expiresPattern       = regexp.MustCompile(`expires=(\d+)`)
)

type Credential struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"projectId"`
	Host      string  `json:"host"`
	Provider  string  `json:"provider"`
	CreatedAt string  `json:"createdAt"`
	ExpiresAt *string `json:"expiresAt"`
}

type SaveResult struct {
	Credential *Credential       `json:"credential,omitempty"`
	Problem    *SignatureProblem `json:"problem,omitempty"`
}

type SignatureProblem struct {
	Reason     string `json:"reason"`
	Host       string `json:"host"`
	SignedHost string `json:"signedHost,omitempty"`
}

type Service struct {
	DB     *pgxpool.Pool
	Secret string
	Client *http.Client
}

func (s *Service) List(ctx context.Context, organizationID string) ([]Credential, error) {
	rows, err := s.DB.Query(ctx, `SELECT c.id,c.project_id,c.host,c.provider,c.created_at,c.expires_at FROM crawler_credentials c JOIN projects p ON p.id=c.project_id WHERE p.organization_id=$1 AND p.archived_at IS NULL ORDER BY c.host`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list crawler credentials: %w", err)
	}
	defer rows.Close()
	out := []Credential{}
	for rows.Next() {
		var item Credential
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.Host, &item.Provider, &item.CreatedAt, &item.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan crawler credential: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate crawler credentials: %w", err)
	}
	return out, nil
}

func (s *Service) Save(ctx context.Context, organizationID, projectID, userID, rawHost, signatureInput, signature string) (SaveResult, error) {
	host, ok := audit.NormalizeCrawlerHost(rawHost)
	if !ok {
		return SaveResult{}, errors.New("invalid host")
	}
	problem, err := s.checkSignature(ctx, host, signatureInput, signature)
	if err != nil {
		return SaveResult{}, err
	}
	if problem != nil {
		return SaveResult{Problem: problem}, nil
	}
	inputCipher, err := encrypt(s.Secret, signatureInput)
	if err != nil {
		return SaveResult{}, err
	}
	signatureCipher, err := encrypt(s.Secret, signature)
	if err != nil {
		return SaveResult{}, err
	}
	id, err := ids.New()
	if err != nil {
		return SaveResult{}, err
	}
	var createdAt string
	var expiresAt *string
	if expiry := expiresPattern.FindStringSubmatch(signatureInput); len(expiry) == 2 {
		if seconds, err := strconv.ParseInt(expiry[1], 10, 64); err == nil {
			value := time.Unix(seconds, 0).UTC().Format(time.RFC3339Nano)
			expiresAt = &value
		}
	}
	err = s.DB.QueryRow(ctx, `INSERT INTO crawler_credentials (id,project_id,host,provider,signature_input,signature,expires_at,created_by_user_id,created_at) SELECT $1,p.id,$3,'shopify',$4,$5,$6,$7,to_char(now() AT TIME ZONE 'utc','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM projects p WHERE p.id=$2 AND p.organization_id=$8 AND p.archived_at IS NULL ON CONFLICT (project_id,host) DO UPDATE SET provider='shopify',signature_input=EXCLUDED.signature_input,signature=EXCLUDED.signature,expires_at=EXCLUDED.expires_at,created_by_user_id=EXCLUDED.created_by_user_id,created_at=EXCLUDED.created_at RETURNING created_at`, id, projectID, host, inputCipher, signatureCipher, expiresAt, userID, organizationID).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SaveResult{}, ErrNotFound
	}
	if err != nil {
		return SaveResult{}, fmt.Errorf("save crawler credential: %w", err)
	}
	var saved Credential
	if err := s.DB.QueryRow(ctx, `SELECT id,project_id,host,provider,created_at,expires_at FROM crawler_credentials WHERE project_id=$1 AND host=$2`, projectID, host).Scan(&saved.ID, &saved.ProjectID, &saved.Host, &saved.Provider, &saved.CreatedAt, &saved.ExpiresAt); err != nil {
		return SaveResult{}, fmt.Errorf("load saved crawler credential: %w", err)
	}
	return SaveResult{Credential: &saved}, nil
}

func (s *Service) Delete(ctx context.Context, organizationID, id string) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM crawler_credentials c USING projects p WHERE c.project_id=p.id AND p.organization_id=$1 AND c.id=$2`, organizationID, id)
	if err != nil {
		return fmt.Errorf("delete crawler credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Resolve(ctx context.Context, organizationID, projectID, host string) (*audit.CrawlerAccess, string, error) {
	host, ok := audit.NormalizeCrawlerHost(host)
	if !ok {
		return nil, "", nil
	}
	rows, err := s.DB.Query(ctx, `SELECT c.id,c.host,c.signature_input,c.signature,c.expires_at FROM crawler_credentials c JOIN projects p ON p.id=c.project_id WHERE p.organization_id=$1 AND p.archived_at IS NULL AND c.host=$2 ORDER BY (c.project_id=$3) DESC,c.created_at DESC`, organizationID, host, projectID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve crawler credential: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, storedHost, inputCipher, signatureCipher string
		var expiry *string
		if err := rows.Scan(&id, &storedHost, &inputCipher, &signatureCipher, &expiry); err != nil {
			return nil, "", fmt.Errorf("scan crawler credential: %w", err)
		}
		input, err := decrypt(s.Secret, inputCipher)
		if err != nil {
			continue
		}
		signature, err := decrypt(s.Secret, signatureCipher)
		if err != nil {
			continue
		}
		var expires *time.Time
		if expiry != nil {
			parsed, err := time.Parse(time.RFC3339Nano, *expiry)
			if err == nil {
				expires = &parsed
			}
		}
		return &audit.CrawlerAccess{Host: storedHost, Headers: audit.ShopifyCrawlerHeaders(input, signature), ExpiresAt: expires}, id, nil
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate crawler credentials: %w", err)
	}
	return nil, "", nil
}

func encrypt(secret, plaintext string) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("BETTER_AUTH_SECRET is too short to encrypt crawler credentials")
	}
	key := sha256.Sum256([]byte(secret))
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return "", err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(aead.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

func decrypt(secret, ciphertext string) (string, error) {
	key := sha256.Sum256([]byte(secret))
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return "", err
	}
	data, err := hex.DecodeString(ciphertext)
	if err != nil || len(data) < chacha20poly1305.NonceSizeX+aead.Overhead() {
		return "", errors.New("invalid crawler credential ciphertext")
	}
	plaintext, err := aead.Open(nil, data[:chacha20poly1305.NonceSizeX], data[chacha20poly1305.NonceSizeX:], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (s *Service) checkSignature(ctx context.Context, host, input, signature string) (*SignatureProblem, error) {
	params := signatureParams.FindStringSubmatch(input)
	keyID := keyIDPattern.FindStringSubmatch(input)
	encoded := signaturePattern.FindStringSubmatch(signature)
	if len(params) != 2 || len(keyID) != 2 || len(encoded) != 2 {
		return &SignatureProblem{Reason: "invalid", Host: host}, nil
	}
	sig, err := base64.StdEncoding.DecodeString(encoded[1])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return &SignatureProblem{Reason: "invalid", Host: host}, nil
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, keyDirectory, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil
	}
	var directory struct {
		Kid string `json:"kid"`
		X   string `json:"x"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&directory); err != nil {
		return nil, nil
	}
	if directory.Kid != keyID[1] {
		return nil, nil
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(directory.X)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, nil
	}
	verify := func(domain string) bool {
		base := fmt.Sprintf("\"@authority\": %s\n\"signature-agent\": %s\n\"@signature-params\": %s", domain, audit.ShopifySignatureAgent, params[1])
		return ed25519.Verify(ed25519.PublicKey(publicKey), []byte(base), sig)
	}
	if verify(host) {
		return nil, nil
	}
	twin := host
	if strings.HasPrefix(host, "www.") {
		twin = strings.TrimPrefix(host, "www.")
	} else {
		twin = "www." + host
	}
	if verify(twin) {
		return &SignatureProblem{Reason: "wrong_domain", Host: host, SignedHost: twin}, nil
	}
	return &SignatureProblem{Reason: "invalid", Host: host}, nil
}
