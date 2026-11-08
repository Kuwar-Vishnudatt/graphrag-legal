package synth

import (
	"strings"
	"testing"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/graph"
)

func TestSynthesizeWithNoContextReportsNotFound(t *testing.T) {
	a := Synthesize("anything", graph.ExpandResult{})
	if !strings.Contains(a.Text, "No relevant sections found") {
		t.Fatalf("expected a not-found message, got %q", a.Text)
	}
}

func TestSynthesizeSurfacesExceptionExplicitly(t *testing.T) {
	primary := document.Section{
		ID: "s1", Title: "General Rule", Text: "90 days retention.",
		Refs: []document.Reference{{TargetID: "s9", Kind: document.RelExceptionTo}},
	}
	exception := document.Section{ID: "s9", Title: "Exception", Text: "7 years for healthcare data."}

	a := Synthesize("q", graph.ExpandResult{
		Sections:    []document.Section{primary, exception},
		HopSections: []document.Section{primary, exception},
	})

	if !strings.Contains(a.Text, "exception") || !strings.Contains(a.Text, "7 years") {
		t.Fatalf("expected the exception to be explicitly surfaced, got %q", a.Text)
	}
	if len(a.Sources) != 2 {
		t.Fatalf("expected both sections cited as sources, got %v", a.Sources)
	}
}

func TestSynthesizeOmitsExceptionWhenNotInContext(t *testing.T) {
	// The critical negative case: if the exception section was never
	// retrieved (e.g. single-hop retrieval stopped before reaching it),
	// the synthesized answer must not mention it — proving an incomplete
	// context genuinely produces an incomplete answer, not one that
	// somehow "knows" about information it was never given.
	primary := document.Section{ID: "s1", Title: "General Rule", Text: "90 days retention."}

	a := Synthesize("q", graph.ExpandResult{
		Sections:    []document.Section{primary},
		HopSections: []document.Section{primary},
	})
	if strings.Contains(a.Text, "7 years") || strings.Contains(a.Text, "exception") {
		t.Fatalf("expected no mention of the exception when it wasn't in context, got %q", a.Text)
	}
}
