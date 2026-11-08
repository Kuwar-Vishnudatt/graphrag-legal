// Package integration proves the actual value proposition of GraphRAG
// against chunk-based top-k retrieval, using the bundled legal corpus
// (testdata/docs/corpus.yaml): a real query whose CORRECT, COMPLETE
// answer requires information that only a multi-hop graph traversal
// reaches, where single-hop (seed-only) retrieval produces a materially
// wrong answer not because synthesis failed, but because the needed
// information was never in its context at all.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/graph"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/retrieval"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/synth"
)

func loadCorpus(t *testing.T) []document.Section {
	t.Helper()
	root := findRepoRoot(t)
	sections, err := document.Load(filepath.Join(root, "testdata", "docs", "corpus.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return sections
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

// TestHealthcareExceptionRequiresMultiHop is the capstone test for the
// whole project. The query's strongest lexical match is the GENERAL
// retention rule (s1) — it shares more exact keywords with the query
// than the healthcare-specific exception section does, which is
// realistic: an exception clause often uses more specific vocabulary
// than the question asking about it. A pure top-k chunk retriever
// returning just the top lexical match would hand a synthesizer ONLY the
// general rule, producing a confidently wrong "90 days" answer for a
// healthcare-specific question. One hop of graph traversal along the
// exception_to edge reaches the actual answer (7 years), and definition
// pull-in additionally surfaces what "Sensitive Data" means — all three
// pieces of information a complete, correct answer needs.
func TestHealthcareExceptionRequiresMultiHop(t *testing.T) {
	sections := loadCorpus(t)
	g := graph.Build(sections)
	query := "What is the data retention period for this regulation, and does it apply to a hospital's patient health records?"

	seeds := retrieval.FindSeeds(sections, query, 1)
	if len(seeds) != 1 {
		t.Fatalf("expected exactly 1 top seed, got %+v", seeds)
	}
	if seeds[0].SectionID != "s1-retention" {
		t.Fatalf("expected the general retention rule to be the strongest single lexical match (that's the whole point of this test), got %q", seeds[0].SectionID)
	}
	seedID := seeds[0].SectionID

	// --- Single-hop (effectively chunk-based top-k): seed only. ---
	zeroHop := g.Expand([]string{seedID}, 0)
	zeroHopAnswer := synth.Synthesize(query, zeroHop)

	if !strings.Contains(zeroHopAnswer.Text, "90 days") {
		t.Fatalf("expected the 0-hop answer to state the general 90-day rule, got %q", zeroHopAnswer.Text)
	}
	if strings.Contains(zeroHopAnswer.Text, "7 years") {
		t.Fatalf("SETUP BUG: 0-hop context should NOT reach the healthcare exception at all — got %q", zeroHopAnswer.Text)
	}

	// --- Multi-hop: 1 hop reaches the exception_to edge, which in turn
	// triggers definition pull-in for "Sensitive Data". ---
	oneHop := g.Expand([]string{seedID}, 1)
	oneHopAnswer := synth.Synthesize(query, oneHop)

	if !strings.Contains(oneHopAnswer.Text, "7 years") {
		t.Fatalf("expected the 1-hop answer to surface the healthcare exception's 7-year period, got %q", oneHopAnswer.Text)
	}
	if !strings.Contains(oneHopAnswer.Text, "Sensitive Data") {
		t.Fatalf("expected the pulled-in definition of Sensitive Data to appear, got %q", oneHopAnswer.Text)
	}

	foundDef := false
	for _, s := range oneHop.Sections {
		if s.ID == "def-sensitive-data" {
			foundDef = true
		}
	}
	if !foundDef {
		t.Fatal("expected def-sensitive-data to be pulled in via definition matching, not graph distance (it has no direct edge to anything)")
	}
}

// TestAmendmentHistoryRequiresFollowingAmendsEdge proves a second,
// independent multi-hop scenario: understanding the CURRENT rule's
// history (that it used to be 30 days, changed by a 2015 amendment)
// requires traversing from the current rule back through the amendment
// chain to the original 1998 section — information a query about "the
// history of this rule" cannot get from retrieving only the
// best-lexical-match current section.
func TestAmendmentHistoryRequiresFollowingAmendsEdge(t *testing.T) {
	sections := loadCorpus(t)
	g := graph.Build(sections)
	query := "How quickly must a company notify people after a data breach, and has this deadline changed over time?"

	seeds := retrieval.FindSeeds(sections, query, 1)
	if len(seeds) != 1 {
		t.Fatalf("expected a seed, got %+v", seeds)
	}

	zeroHop := g.Expand([]string{seeds[0].SectionID}, 0)
	twoHop := g.Expand([]string{seeds[0].SectionID}, 2)

	zeroIDs := map[string]bool{}
	for _, s := range zeroHop.Sections {
		zeroIDs[s.ID] = true
	}
	twoIDs := map[string]bool{}
	for _, s := range twoHop.Sections {
		twoIDs[s.ID] = true
	}

	if zeroIDs["s4-original-breach-window"] {
		t.Fatal("SETUP BUG: 0-hop should not already reach the original 1998 section")
	}
	if !twoIDs["s4-original-breach-window"] {
		t.Fatalf("expected 2-hop traversal to reach the original 1998 section through the amendment chain, got sections: %v", twoHop.Sections)
	}
}
