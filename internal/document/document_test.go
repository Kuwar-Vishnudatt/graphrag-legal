package document

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCorpus(t *testing.T, yaml string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "corpus.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidCorpus(t *testing.T) {
	path := writeCorpus(t, `
sections:
  - id: s1
    title: Retention
    text: Data must be retained for no more than 90 days.
    refs:
      - target: s2
        kind: exception_to
  - id: s2
    title: Healthcare Exception
    text: Section s1 does not apply to healthcare data.
`)
	sections, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
}

func TestLoadRejectsDuplicateIDs(t *testing.T) {
	path := writeCorpus(t, `
sections:
  - id: s1
    title: A
    text: t
  - id: s1
    title: B
    text: t
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for duplicate section ids")
	}
}

func TestLoadRejectsDanglingReference(t *testing.T) {
	path := writeCorpus(t, `
sections:
  - id: s1
    title: A
    text: t
    refs:
      - target: does-not-exist
        kind: references
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for a reference to a nonexistent section")
	}
}

func TestLoadRejectsUnknownReferenceKind(t *testing.T) {
	path := writeCorpus(t, `
sections:
  - id: s1
    title: A
    text: t
    refs:
      - target: s2
        kind: sideways
  - id: s2
    title: B
    text: t
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for an unknown reference kind")
	}
}

func TestLoadRejectsMissingID(t *testing.T) {
	path := writeCorpus(t, `
sections:
  - title: A
    text: t
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error for a section missing an id")
	}
}
