// Package synth turns a query and an assembled graph context into an
// answer. It is deliberately simple (concatenation with provenance, not
// real language generation) — the correctness property this project
// proves is about what makes it INTO the context (internal/graph's job),
// not how well an answer is phrased from it. A real deployment would
// swap this for an LLM call; the synthesizer only ever sees what the
// graph expansion handed it, so a wrong or incomplete answer here is
// traceable to a retrieval gap, not a generation failure — exactly the
// distinction the capstone test in internal/graph's package (see
// docs/ARCHITECTURE.md) is built to isolate.
package synth

import (
	"fmt"
	"strings"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/graph"
)

type Answer struct {
	Text    string
	Sources []string // section IDs actually cited
}

// Synthesize builds an answer entirely from expansion's assembled
// context — every exception_to and amends relationship present in that
// context is surfaced explicitly (not left for the reader to infer),
// since silently omitting "section X overrides this" is exactly the kind
// of incomplete-looking-complete answer a real legal reader can't afford.
func Synthesize(query string, expansion graph.ExpandResult) Answer {
	if len(expansion.Sections) == 0 {
		return Answer{Text: fmt.Sprintf("No relevant sections found for %q.", query)}
	}

	byID := make(map[string]document.Section, len(expansion.Sections))
	for _, s := range expansion.Sections {
		byID[s.ID] = s
	}

	var b strings.Builder
	var sources []string
	seen := map[string]bool{}

	primary := expansion.Sections[0]
	fmt.Fprintf(&b, "%s (%s): %s", primary.Title, primary.ID, primary.Text)
	sources = append(sources, primary.ID)
	seen[primary.ID] = true

	for _, ref := range primary.Refs {
		target, ok := byID[ref.TargetID]
		if !ok || seen[ref.TargetID] {
			continue
		}
		switch ref.Kind {
		case document.RelExceptionTo:
			fmt.Fprintf(&b, "\n\nHowever, %s (%s) establishes an exception: %s", target.Title, target.ID, target.Text)
		case document.RelAmends:
			fmt.Fprintf(&b, "\n\nNote: %s (%s) amends this: %s", target.Title, target.ID, target.Text)
		case document.RelReferences:
			fmt.Fprintf(&b, "\n\nRelated: %s (%s): %s", target.Title, target.ID, target.Text)
		}
		sources = append(sources, target.ID)
		seen[target.ID] = true
	}

	for _, s := range expansion.Sections {
		if seen[s.ID] {
			continue
		}
		fmt.Fprintf(&b, "\n\nDefinition — %s (%s): %s", s.Title, s.ID, s.Text)
		sources = append(sources, s.ID)
		seen[s.ID] = true
	}

	return Answer{Text: b.String(), Sources: sources}
}
