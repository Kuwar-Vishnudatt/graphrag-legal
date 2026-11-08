// Package document defines the corpus this project reasons over: legal/
// regulatory sections with EXPLICIT typed cross-references, rather than
// references inferred by NLP. Real citation extraction from legal text
// (resolving "see subsection (b) above" or "as defined in the preceding
// Article" into a concrete target) is a serious, separate NLP problem;
// this project's engineering content is the graph construction and
// multi-hop traversal over a citation graph, not citation extraction
// itself — see docs/ARCHITECTURE.md. Explicit reference markup lets every
// downstream package be tested deterministically against a known-correct
// graph shape.
package document

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RelationKind is the type of a directed edge between two sections. The
// kind matters for traversal policy (internal/graph) — not every question
// should follow every kind of edge; see docs/ARCHITECTURE.md.
type RelationKind string

const (
	RelReferences  RelationKind = "references"   // generic cross-reference ("see Section X")
	RelAmends      RelationKind = "amends"       // this section modifies the effect of the target
	RelExceptionTo RelationKind = "exception_to" // this section carves out an exception to the target
)

// Reference is one outgoing typed edge from a section.
type Reference struct {
	TargetID string       `yaml:"target"`
	Kind     RelationKind `yaml:"kind"`
}

// Section is one unit of the corpus — a section, article, or clause.
type Section struct {
	ID    string      `yaml:"id"`
	Title string      `yaml:"title"`
	Text  string      `yaml:"text"`
	Refs  []Reference `yaml:"refs"`
	// DefinedTerms are terms this section defines (its text is the
	// authoritative definition) — looked up by internal/graph whenever
	// another section's text uses one of these terms, regardless of hop
	// distance; see docs/ARCHITECTURE.md for why definitions are handled
	// differently from the bounded-hop traversal used for the Kind edges
	// above.
	DefinedTerms []string `yaml:"defined_terms,omitempty"`
}

type corpusFile struct {
	Sections []Section `yaml:"sections"`
}

// Load reads a corpus YAML file and validates its basic shape: every
// section has a unique ID, and every reference points at a section that
// actually exists in this corpus (a dangling reference is a corpus
// authoring bug, not something downstream code should have to tolerate).
func Load(path string) ([]Section, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus %s: %w", path, err)
	}
	var cf corpusFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse corpus %s: %w", path, err)
	}

	ids := make(map[string]bool, len(cf.Sections))
	for _, s := range cf.Sections {
		if s.ID == "" {
			return nil, fmt.Errorf("corpus %s: a section is missing an id", path)
		}
		if ids[s.ID] {
			return nil, fmt.Errorf("corpus %s: duplicate section id %q", path, s.ID)
		}
		ids[s.ID] = true
	}
	for _, s := range cf.Sections {
		for _, ref := range s.Refs {
			if !ids[ref.TargetID] {
				return nil, fmt.Errorf("corpus %s: section %q references unknown section %q", path, s.ID, ref.TargetID)
			}
			switch ref.Kind {
			case RelReferences, RelAmends, RelExceptionTo:
			default:
				return nil, fmt.Errorf("corpus %s: section %q has an unknown reference kind %q", path, s.ID, ref.Kind)
			}
		}
	}

	return cf.Sections, nil
}
