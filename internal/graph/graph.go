// Package graph builds a citation graph over a document.Section corpus
// and answers the question chunk-based top-k retrieval structurally
// cannot: "what else do I need to read to fully understand this
// section." Two distinct mechanisms contribute to that, deliberately kept
// separate:
//
//   - Bounded-hop traversal over typed reference edges (references,
//     amends, exception_to) — "what's connected to this, within N hops."
//   - Definition pull-in — whenever a collected section's text uses a
//     term the corpus defines elsewhere, that definition is included
//     regardless of hop distance, because you cannot correctly understand
//     a section that uses an undefined term no matter how "close" you
//     consider it to be.
package graph

import (
	"regexp"
	"strings"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
)

// Graph is a built, queryable citation graph.
type Graph struct {
	sections map[string]document.Section
	// neighbors is the traversal adjacency list, built from BOTH a
	// section's own outgoing Refs AND every other section's incoming
	// reference TO it — see Build's doc comment for why traversal is
	// bidirectional even though Reference.Kind is directional.
	neighbors map[string][]string
	// definitions maps a defined term (lowercased) to the section ID that
	// defines it.
	definitions map[string]string
	// termPattern matches any defined term as a whole word/phrase,
	// case-insensitively — built once at construction time so Expand
	// doesn't rebuild a regexp per call.
	termPatterns map[string]*regexp.Regexp
}

// Build constructs a Graph from a loaded corpus.
//
// Traversal follows edges in BOTH directions, not just a section's own
// outgoing Refs. This is deliberate, and was found to matter via an
// actual failing test (TestAmendmentHistoryRequiresFollowingAmendsEdge):
// a 2015 amendment section naturally references back to the 1998 section
// it amends (an outgoing edge FROM the amendment TO the original), but
// the original 1998 section has no reason to reference forward to its
// own future amendment — it predates it. A query seeded on the CURRENT
// rule asking about its history needs to walk backward along that same
// edge (the amendment references the original) even though the current
// rule itself has no outgoing edge there. Reference.Kind is preserved
// and still carries real directional meaning for explanation purposes
// (internal/synth); only the connectivity used for traversal purposes is
// undirected.
func Build(sections []document.Section) *Graph {
	g := &Graph{
		sections:     make(map[string]document.Section, len(sections)),
		neighbors:    make(map[string][]string),
		definitions:  make(map[string]string),
		termPatterns: make(map[string]*regexp.Regexp),
	}
	for _, s := range sections {
		g.sections[s.ID] = s
		for _, term := range s.DefinedTerms {
			key := strings.ToLower(term)
			g.definitions[key] = s.ID
			g.termPatterns[key] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(term) + `\b`)
		}
	}
	for _, s := range sections {
		for _, ref := range s.Refs {
			g.neighbors[s.ID] = append(g.neighbors[s.ID], ref.TargetID)
			g.neighbors[ref.TargetID] = append(g.neighbors[ref.TargetID], s.ID)
		}
	}
	return g
}

// ExpandResult is the full context a multi-hop query assembled.
type ExpandResult struct {
	// Sections is every section included in the final context, in the
	// order first discovered (seeds first, then BFS order, then
	// definitions pulled in after).
	Sections []document.Section
	// HopSections are the sections reached purely by bounded-hop
	// traversal, excluding definitions pulled in afterward — useful for
	// a caller (or a test) that wants to distinguish "reached by
	// following references" from "pulled in because it defines a term in
	// use."
	HopSections []document.Section
}

// Expand performs bounded-hop BFS from seedIDs over every reference edge
// kind, up to maxHops, then pulls in definitions for any defined term
// used anywhere in the collected text — recursively, since a definition's
// own text may use another defined term — until a fixed point (no new
// section added in a pass).
//
// maxHops=0 means "seeds only, no traversal" (still runs definition
// pull-in on the seeds themselves) — this is what lets
// TestMultiHopFindsWhatSingleHopMisses compare "seed-only" against
// "seed+traversal" using the exact same function, not two different code
// paths.
func (g *Graph) Expand(seedIDs []string, maxHops int) ExpandResult {
	visited := make(map[string]bool)
	var hopOrder []document.Section

	type queued struct {
		id   string
		hops int
	}
	queue := make([]queued, 0, len(seedIDs))
	for _, id := range seedIDs {
		if _, ok := g.sections[id]; !ok {
			continue
		}
		queue = append(queue, queued{id: id, hops: 0})
		visited[id] = true
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		sec := g.sections[cur.id]
		hopOrder = append(hopOrder, sec)

		if cur.hops >= maxHops {
			continue // don't expand past the hop limit, but the node itself is still included
		}
		for _, neighborID := range g.neighbors[sec.ID] {
			if visited[neighborID] {
				continue // cycle-safe: a section referencing back toward something already queued is a no-op, not a re-enqueue
			}
			visited[neighborID] = true
			queue = append(queue, queued{id: neighborID, hops: cur.hops + 1})
		}
	}

	allSections := append([]document.Section(nil), hopOrder...)
	included := make(map[string]bool, len(allSections))
	for _, s := range allSections {
		included[s.ID] = true
	}

	// Definition pull-in: repeat until a pass adds nothing new.
	for {
		addedThisPass := false
		for _, s := range allSections {
			for term, pattern := range g.termPatterns {
				defSectionID := g.definitions[term]
				if included[defSectionID] {
					continue
				}
				if pattern.MatchString(s.Text) {
					defSec := g.sections[defSectionID]
					allSections = append(allSections, defSec)
					included[defSectionID] = true
					addedThisPass = true
				}
			}
		}
		if !addedThisPass {
			break
		}
	}

	return ExpandResult{Sections: allSections, HopSections: hopOrder}
}

// Section looks up one section by ID.
func (g *Graph) Section(id string) (document.Section, bool) {
	s, ok := g.sections[id]
	return s, ok
}
