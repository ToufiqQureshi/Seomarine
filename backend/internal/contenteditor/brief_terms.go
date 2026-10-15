package contenteditor

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strings"
)

// Limits for the terms and subtopics a brief derives from competitor pages.
const (
	briefMaxTerms     = 20
	briefMaxSubtopics = 8
)

var briefWordPattern = regexp.MustCompile(`[\p{L}\p{N}]+(?:['’][\p{L}]+)?`)

// briefStopWords are left out of terms and subtopics: they appear on every
// page and say nothing about the topic. Common Hinglish fillers are included
// because Indian pages mix them into English text.
var briefStopWords = func() map[string]bool {
	words := strings.Fields(`a about above after again against all also am an and any are as at be because been before being below between both but by can could did do does doing down during each few for from further had has have having he her here hers him his how i if in into is it its itself just me more most my no nor not now of off on once only or other our ours out over own same she should so some such than that the their them then there these they this those through to too under until up very was we were what when where which while who whom why will with would you your yours
		get got make made many much may might must need new one per two use used using via way well like see also every first last next best top know want here's it's don't can't won't
		hai hain ka ki ke se mein me aur ko par yeh ye kya kaise nahi nahin hum aap ek bhi tak toh to jo`)
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}()

func briefWords(text string) []string {
	return briefWordPattern.FindAllString(strings.ToLower(text), -1)
}

func isBriefContentWord(word string) bool {
	if len([]rune(word)) < 3 || briefStopWords[word] {
		return false
	}
	return strings.IndexFunc(word, func(r rune) bool { return r < '0' || r > '9' }) >= 0
}

// briefPhraseCounts counts content unigrams and bigrams of content words.
func briefPhraseCounts(text string) map[string]int {
	counts := map[string]int{}
	words := briefWords(text)
	for i, word := range words {
		if !isBriefContentWord(word) {
			continue
		}
		counts[word]++
		if i+1 < len(words) && isBriefContentWord(words[i+1]) {
			counts[word+" "+words[i+1]]++
		}
	}
	return counts
}

type briefCandidate struct {
	phrase  string
	domains int
	total   int
	perPage []int
}

// briefTerms returns the phrases most competitor pages use, ranked by how
// many pages use them (not raw frequency, so one long page cannot dominate),
// with a use range from the pages that use them: the 25th and 75th
// percentiles. A unigram already inside a chosen two-word term is skipped.
func briefTerms(competitors []BriefCompetitor) []BriefTerm {
	minDomains := max(2, int(math.Ceil(float64(len(competitors))*0.4)))
	candidates := rankBriefPhrases(competitors, func(c BriefCompetitor) string { return c.Text }, minDomains)
	terms := make([]BriefTerm, 0, briefMaxTerms)
	for _, candidate := range candidates {
		if len(terms) == briefMaxTerms {
			break
		}
		if !strings.Contains(candidate.phrase, " ") && slices.ContainsFunc(terms, func(term BriefTerm) bool {
			return slices.Contains(strings.Fields(term.Term), candidate.phrase)
		}) {
			continue
		}
		slices.Sort(candidate.perPage)
		low := candidate.perPage[int(float64(len(candidate.perPage)-1)*0.25)]
		high := candidate.perPage[int(math.Ceil(float64(len(candidate.perPage)-1)*0.75))]
		terms = append(terms, BriefTerm{Term: candidate.phrase, Min: max(1, low), Max: max(1, low, high), Domains: candidate.domains})
	}
	return terms
}

// briefSubtopics returns short phrases that several competitors use in their
// headings ("crawl budget", "site speed"). The editor checks whether the
// writer's H2s mention them.
func briefSubtopics(competitors []BriefCompetitor) []string {
	candidates := rankBriefPhrases(competitors, func(c BriefCompetitor) string { return strings.Join(c.Headings, "\n") }, 2)
	subtopics := make([]string, 0, briefMaxSubtopics)
	for _, candidate := range candidates {
		if len(subtopics) == briefMaxSubtopics {
			break
		}
		if !strings.Contains(candidate.phrase, " ") && slices.ContainsFunc(subtopics, func(topic string) bool {
			return slices.Contains(strings.Fields(topic), candidate.phrase)
		}) {
			continue
		}
		subtopics = append(subtopics, candidate.phrase)
	}
	return subtopics
}

func rankBriefPhrases(competitors []BriefCompetitor, text func(BriefCompetitor) string, minDomains int) []briefCandidate {
	byPhrase := map[string]*briefCandidate{}
	for _, competitor := range competitors {
		for phrase, count := range briefPhraseCounts(text(competitor)) {
			candidate := byPhrase[phrase]
			if candidate == nil {
				candidate = &briefCandidate{phrase: phrase}
				byPhrase[phrase] = candidate
			}
			candidate.domains++
			candidate.total += count
			candidate.perPage = append(candidate.perPage, count)
		}
	}
	ranked := make([]briefCandidate, 0, len(byPhrase))
	for _, candidate := range byPhrase {
		if candidate.domains >= minDomains {
			ranked = append(ranked, *candidate)
		}
	}
	slices.SortFunc(ranked, func(a, b briefCandidate) int {
		return cmp.Or(
			cmp.Compare(b.domains, a.domains),
			// Two-word phrases are more specific than single words.
			cmp.Compare(strings.Count(b.phrase, " "), strings.Count(a.phrase, " ")),
			cmp.Compare(b.total, a.total),
			cmp.Compare(a.phrase, b.phrase),
		)
	})
	return ranked
}
