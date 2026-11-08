// Package retrieval finds the seed section(s) a query should start graph
// traversal from. Lexical word-overlap scoring, deliberately — the
// engineering content this project is about is the graph traversal and
// definition pull-in happening AFTER a seed is found, not retrieval
// sophistication; see docs/ARCHITECTURE.md.
package retrieval

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
)

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true, "were": true,
	"be": true, "been": true, "of": true, "to": true, "in": true, "on": true, "at": true,
	"for": true, "with": true, "by": true, "this": true, "that": true, "it": true, "its": true,
	"and": true, "or": true, "but": true, "what": true, "how": true, "can": true, "does": true,
	"do": true, "under": true, "s": true,
}

func words(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		if !stopWords[w] {
			m[w] = true
		}
	}
	return m
}

func overlapScore(a, b map[string]bool) int {
	n := 0
	for w := range a {
		if b[w] {
			n++
		}
	}
	return n
}

// Seed is one candidate seed section with its lexical score.
type Seed struct {
	SectionID string
	Score     int
}

// FindSeeds ranks every section by word overlap with query and returns
// the topN highest-scoring section IDs (sections with zero overlap are
// excluded even if that means fewer than topN results).
func FindSeeds(sections []document.Section, query string, topN int) []Seed {
	qWords := words(query)

	var scored []Seed
	for _, s := range sections {
		score := overlapScore(qWords, words(s.Title+" "+s.Text))
		if score > 0 {
			scored = append(scored, Seed{SectionID: s.ID, Score: score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

	if topN > len(scored) {
		topN = len(scored)
	}
	return scored[:topN]
}
