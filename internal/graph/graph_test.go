package graph

import (
	"testing"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
)

func sec(id, title, text string, refs ...document.Reference) document.Section {
	return document.Section{ID: id, Title: title, Text: text, Refs: refs}
}

func ref(target string, kind document.RelationKind) document.Reference {
	return document.Reference{TargetID: target, Kind: kind}
}

func idsOf(sections []document.Section) map[string]bool {
	m := make(map[string]bool, len(sections))
	for _, s := range sections {
		m[s.ID] = true
	}
	return m
}

func TestExpandZeroHopsReturnsOnlySeed(t *testing.T) {
	g := Build([]document.Section{
		sec("s1", "A", "text a", ref("s2", document.RelReferences)),
		sec("s2", "B", "text b"),
	})
	res := g.Expand([]string{"s1"}, 0)
	ids := idsOf(res.Sections)
	if len(ids) != 1 || !ids["s1"] {
		t.Fatalf("expected only the seed at 0 hops, got %v", ids)
	}
}

func TestExpandOneHopReachesDirectNeighbor(t *testing.T) {
	g := Build([]document.Section{
		sec("s1", "A", "text a", ref("s2", document.RelExceptionTo)),
		sec("s2", "B", "text b"),
	})
	res := g.Expand([]string{"s1"}, 1)
	ids := idsOf(res.Sections)
	if !ids["s1"] || !ids["s2"] {
		t.Fatalf("expected both s1 and s2 at 1 hop, got %v", ids)
	}
}

func TestExpandDoesNotReachBeyondHopLimit(t *testing.T) {
	g := Build([]document.Section{
		sec("s1", "A", "a", ref("s2", document.RelReferences)),
		sec("s2", "B", "b", ref("s3", document.RelReferences)),
		sec("s3", "C", "c"),
	})
	res := g.Expand([]string{"s1"}, 1)
	ids := idsOf(res.Sections)
	if ids["s3"] {
		t.Fatalf("expected s3 to be unreachable at 1 hop (it's 2 hops away), got %v", ids)
	}
	if !ids["s2"] {
		t.Fatal("expected s2 to be reached at 1 hop")
	}
}

func TestExpandTwoHopsReachesTransitiveNeighbor(t *testing.T) {
	g := Build([]document.Section{
		sec("s1", "A", "a", ref("s2", document.RelReferences)),
		sec("s2", "B", "b", ref("s3", document.RelReferences)),
		sec("s3", "C", "c"),
	})
	res := g.Expand([]string{"s1"}, 2)
	ids := idsOf(res.Sections)
	if !ids["s1"] || !ids["s2"] || !ids["s3"] {
		t.Fatalf("expected all 3 sections reachable at 2 hops, got %v", ids)
	}
}

// TestExpandHandlesCircularReferencesWithoutHanging is the cycle-safety
// test: a real and plausible shape in legal text (a general rule and its
// exception referencing each other back and forth). A BFS without a
// visited-before-enqueue check would loop forever re-queuing s1 and s2;
// this test has an implicit timeout via `go test`'s own deadline, so a
// genuine infinite loop here fails the whole test run, not just this case.
func TestExpandHandlesCircularReferencesWithoutHanging(t *testing.T) {
	g := Build([]document.Section{
		sec("s1", "General Rule", "the rule", ref("s2", document.RelExceptionTo)),
		sec("s2", "Exception", "the exception", ref("s1", document.RelReferences)),
	})
	res := g.Expand([]string{"s1"}, 5)
	ids := idsOf(res.Sections)
	if len(ids) != 2 || !ids["s1"] || !ids["s2"] {
		t.Fatalf("expected exactly s1 and s2, no duplicates, got %v", res.Sections)
	}
}

func TestExpandNeverDuplicatesANodeReachableByTwoPaths(t *testing.T) {
	// s1 -> s2 -> s4 and s1 -> s3 -> s4: s4 is reachable by two distinct
	// paths and must appear exactly once in the result.
	g := Build([]document.Section{
		sec("s1", "A", "a", ref("s2", document.RelReferences), ref("s3", document.RelReferences)),
		sec("s2", "B", "b", ref("s4", document.RelReferences)),
		sec("s3", "C", "c", ref("s4", document.RelReferences)),
		sec("s4", "D", "d"),
	})
	res := g.Expand([]string{"s1"}, 3)
	count := 0
	for _, s := range res.Sections {
		if s.ID == "s4" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected s4 exactly once despite two reachable paths, got %d times", count)
	}
}

func TestDefinitionPullInRegardlessOfHopDistance(t *testing.T) {
	far := sec("far", "Definition", "'Sensitive Data' means health, biometric, or genetic information.")
	far.DefinedTerms = []string{"Sensitive Data"}

	g := Build([]document.Section{
		sec("s1", "Retention Rule", "Sensitive Data must be deleted within 30 days."),
		far,
	})
	// 0 hops: "far" isn't connected by any reference edge at all, but its
	// term appears in s1's text, so it must still be pulled in.
	res := g.Expand([]string{"s1"}, 0)
	ids := idsOf(res.Sections)
	if !ids["far"] {
		t.Fatalf("expected the definition to be pulled in regardless of hop distance (there's no edge at all), got %v", ids)
	}
}

func TestDefinitionPullInIsTransitive(t *testing.T) {
	// s1 uses "Sensitive Data"; its definition uses "Biometric
	// Identifier"; that definition must ALSO be pulled in, even though
	// it's two definition-hops removed from s1.
	defA := sec("def-sensitive", "Def A", "'Sensitive Data' includes any Biometric Identifier.")
	defA.DefinedTerms = []string{"Sensitive Data"}
	defB := sec("def-biometric", "Def B", "'Biometric Identifier' means a fingerprint or retina scan.")
	defB.DefinedTerms = []string{"Biometric Identifier"}

	g := Build([]document.Section{
		sec("s1", "Rule", "Sensitive Data must be encrypted."),
		defA,
		defB,
	})
	res := g.Expand([]string{"s1"}, 0)
	ids := idsOf(res.Sections)
	if !ids["def-sensitive"] || !ids["def-biometric"] {
		t.Fatalf("expected both definitions pulled in transitively, got %v", ids)
	}
}

func TestDefinitionTermMatchingIsWholeWordNotSubstring(t *testing.T) {
	// "Data" as a defined term must not spuriously match inside
	// "Database" or "Dataset" — a substring match would over-include.
	defData := sec("def-data", "Def", "'Data' means any recorded information.")
	defData.DefinedTerms = []string{"Data"}

	g := Build([]document.Section{
		sec("s1", "Rule", "The Database must be backed up nightly."), // "Data" is a substring of "Database" here
		defData,
	})
	res := g.Expand([]string{"s1"}, 0)
	ids := idsOf(res.Sections)
	if ids["def-data"] {
		t.Fatal("expected whole-word matching to NOT pull in a definition merely because the term is a substring of another word")
	}
}

func TestExpandSkipsUnknownSeedIDsGracefully(t *testing.T) {
	g := Build([]document.Section{sec("s1", "A", "a")})
	res := g.Expand([]string{"does-not-exist", "s1"}, 1)
	ids := idsOf(res.Sections)
	if len(ids) != 1 || !ids["s1"] {
		t.Fatalf("expected only the valid seed, unknown ones silently skipped, got %v", ids)
	}
}

// TestExpandFollowsReferencesBackwardNotJustForward is a regression test
// for a real bug found via the integration test's "history of this rule"
// scenario: a current section (s3) is referenced BY an amendment section
// (s5), but s3 itself has no outgoing edge to s5 — it predates the
// amendment and has no reason to reference forward to it. A naive
// forward-only BFS starting from s3 would never reach s5 (or, from
// there, the original section s5 amends), even though understanding s3's
// own history clearly requires exactly that traversal.
func TestExpandFollowsReferencesBackwardNotJustForward(t *testing.T) {
	g := Build([]document.Section{
		sec("s3-current", "Current Rule", "72 hour breach notification."),
		sec("s4-original", "Original Rule", "30 day breach notification.", ref("s5-amendment", document.RelReferences)),
		sec("s5-amendment", "Amendment", "This amends s4, reflected now in s3.",
			ref("s4-original", document.RelAmends), ref("s3-current", document.RelReferences)),
	})

	res := g.Expand([]string{"s3-current"}, 2)
	ids := idsOf(res.Sections)
	if !ids["s5-amendment"] {
		t.Fatalf("expected 1 backward hop to reach the amendment section that references s3, got %v", ids)
	}
	if !ids["s4-original"] {
		t.Fatalf("expected 2 hops to reach the original section through the amendment chain, got %v", ids)
	}
}

func TestHopSectionsExcludesDefinitionPullIns(t *testing.T) {
	farDef := sec("far-def", "Def", "'Thing' means something.")
	farDef.DefinedTerms = []string{"Thing"}

	g := Build([]document.Section{
		sec("s1", "Rule", "A Thing must be reported.", ref("s2", document.RelReferences)),
		sec("s2", "Related", "more text"),
		farDef,
	})
	res := g.Expand([]string{"s1"}, 1)

	hopIDs := idsOf(res.HopSections)
	if hopIDs["far-def"] {
		t.Fatal("expected HopSections to exclude definition pull-ins")
	}
	allIDs := idsOf(res.Sections)
	if !allIDs["far-def"] {
		t.Fatal("expected Sections (the full context) to include the definition pull-in")
	}
}
