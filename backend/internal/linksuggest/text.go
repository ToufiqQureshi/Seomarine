package linksuggest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

// stopwords are the filler words that carry no topic meaning in anchor
// matching; Hindi/Hinglish fillers are included because Uplumo's writers
// mix languages.
var stopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "of": {}, "to": {}, "in": {},
	"on": {}, "for": {}, "with": {}, "is": {}, "are": {}, "be": {}, "your": {},
	"you": {}, "how": {}, "what": {}, "its": {}, "it": {}, "that": {}, "this": {},
	"hai": {}, "hain": {}, "ka": {}, "ki": {}, "ke": {}, "se": {}, "mein": {},
	"me": {}, "aur": {}, "ko": {}, "par": {}, "kya": {}, "nahi": {}, "nahin": {},
}

// uniqueWords tokenizes text into the distinct meaningful words both parts
// of a match must share. Unicode letters and digits stay, so Devanagari
// anchors work; case is folded with the Unicode case rules.
func uniqueWords(text string) []string {
	var words []string
	seen := map[string]struct{}{}
	start := -1
	runes := []rune(strings.ToLower(text))
	// Keep combining marks with their base letters, so Hindi vowel signs do
	// not split a word into unrelated fragments.
	for i, r := range append(runes, ' ') {
		isWord := isWordRune(r)
		switch {
		case isWord && start < 0:
			start = i
		case !isWord && start >= 0:
			word := string(runes[start:i])
			start = -1
			if utf8.RuneCountInString(word) >= 2 {
				if _, stop := stopwords[word]; stop {
					continue
				}
				if _, dup := seen[word]; dup {
					continue
				}
				seen[word] = struct{}{}
				words = append(words, word)
			}
		}
	}
	return words
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

// cacheKey hashes the draft (not its identity), so two drafts with the same
// text share a cached answer and a hash collision cannot leak another
// project's data: the project id is part of the key.
func cacheKey(projectID string, input Input) (string, error) {
	hasher := sha256.New()
	// Hash in a fixed order; a newline-rich content cannot confuse the
	// fields because lengths are written first.
	for _, field := range []string{projectID, input.Content, input.Topic, input.Title, input.Slug} {
		_, err := fmt.Fprintf(hasher, "%d:", len([]rune(field)))
		if err != nil {
			return "", fmt.Errorf("hash link suggestion input: %w", err)
		}
		_, _ = hasher.Write([]byte(field))
	}
	return "linksuggest:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

// encodeCache/decodeCache keep the cache answer as compact JSON; a broken
// payload is treated as a miss, never as an error answer.
func encodeCache(suggestions []Suggestion) string {
	payload, err := json.Marshal(suggestions)
	if err != nil {
		return ""
	}
	return string(payload)
}

// errCacheMiss reports an unusable or absent cache answer.
var errCacheMiss = errors.New("link suggestion cache miss")

func decodeCache(payload string) []Suggestion {
	var suggestions []Suggestion
	if json.Unmarshal([]byte(payload), &suggestions) != nil || suggestions == nil {
		return nil
	}
	return suggestions
}

// RedisCache adapts go-redis to the Service's cache interface.
type RedisCache struct{ Client *redis.Client }

// Get returns errCacheMiss when the key is absent.
func (c RedisCache) Get(ctx context.Context, key string) (string, error) {
	value, err := c.Client.Get(ctx, key).Result()
	switch {
	case errors.Is(err, redis.Nil):
		return "", errCacheMiss
	case err != nil:
		return "", err
	case value == "":
		return "", errCacheMiss
	}
	return value, nil
}

// Set stores a computed suggestion result for the bounded cache lifetime.
func (c RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.Client.Set(ctx, key, value, ttl).Err()
}
