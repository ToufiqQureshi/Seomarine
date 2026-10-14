package sitepublish

import (
	"regexp"
	"slices"
	"strings"
)

// These mirror the editor's no-brief stuffing rule in
// frontend/src/client/features/content-editor/scoring-caps.ts, so the server
// refuses exactly what the editor's "Publish now" button refuses. Change both
// together.
var (
	wordPattern  = regexp.MustCompile(`[\p{L}\p{N}]+(?:['’-][\p{L}\p{N}]+)*`)
	tokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

// keywordStuffed reports whether body repeats topic so often that it reads as
// keyword stuffing: at least 8 uses, more than twice a natural 2% of the
// words, and over 4% of all words.
func keywordStuffed(topic, body string) bool {
	topicTokens := tokenPattern.FindAllString(strings.ToLower(topic), -1)
	if len(topicTokens) == 0 {
		return false
	}
	bodyTokens := tokenPattern.FindAllString(strings.ToLower(body), -1)
	uses := 0
	for i := 0; i+len(topicTokens) <= len(bodyTokens); i++ {
		if slices.Equal(bodyTokens[i:i+len(topicTokens)], topicTokens) {
			uses++
		}
	}
	words := len(wordPattern.FindAllString(body, -1))
	natural := (words*2 + 99) / 100 // ceil(2% of words)
	return uses >= 8 && uses > natural*2 && float64(uses) > 0.04*float64(max(1, words))
}
