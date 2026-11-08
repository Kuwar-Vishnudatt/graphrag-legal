package retrieval

import (
	"testing"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
)

func TestFindSeedsRanksBestOverlapFirst(t *testing.T) {
	sections := []document.Section{
		{ID: "s1", Title: "Retention", Text: "This regulation establishes a general data retention period of 90 days."},
		{ID: "s2", Title: "Unrelated", Text: "Employees must complete annual training."},
	}
	seeds := FindSeeds(sections, "What is the retention period for data?", 5)
	if len(seeds) != 1 || seeds[0].SectionID != "s1" {
		t.Fatalf("expected only s1 to match, got %+v", seeds)
	}
}

func TestFindSeedsExcludesZeroOverlap(t *testing.T) {
	sections := []document.Section{
		{ID: "s1", Title: "A", Text: "completely unrelated content about staffing"},
	}
	seeds := FindSeeds(sections, "data retention period", 5)
	if len(seeds) != 0 {
		t.Fatalf("expected no seeds for zero overlap, got %+v", seeds)
	}
}

func TestFindSeedsRespectsTopN(t *testing.T) {
	sections := []document.Section{
		{ID: "s1", Title: "A", Text: "data retention period rules"},
		{ID: "s2", Title: "B", Text: "data retention period guidance"},
		{ID: "s3", Title: "C", Text: "data retention period notes"},
	}
	seeds := FindSeeds(sections, "data retention period", 2)
	if len(seeds) != 2 {
		t.Fatalf("expected exactly 2 seeds (topN), got %d", len(seeds))
	}
}
