// Command graphrag is the CLI: `ask` runs a query against a legal/
// regulatory corpus at a chosen hop depth, printing the assembled
// synthesized answer — so the single-hop-vs-multi-hop difference this
// project is about can be seen directly, not just read about.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/document"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/graph"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/retrieval"
	"github.com/Kuwar-Vishnudatt/graphrag-legal/internal/synth"
)

func main() {
	fs := flag.NewFlagSet("ask", flag.ExitOnError)
	corpusPath := fs.String("corpus", "testdata/docs/corpus.yaml", "path to the corpus YAML file")
	query := fs.String("query", "", "the question to ask (required)")
	hops := fs.Int("hops", 1, "how many hops of graph traversal to allow (0 = seed section only)")
	topSeeds := fs.Int("seeds", 1, "how many top lexical matches to seed traversal from")
	fs.Parse(os.Args[1:])

	if *query == "" {
		fmt.Fprintln(os.Stderr, "usage: graphrag -query \"...\" [-corpus path] [-hops N] [-seeds N]")
		os.Exit(2)
	}

	sections, err := document.Load(*corpusPath)
	must(err)

	g := graph.Build(sections)
	seeds := retrieval.FindSeeds(sections, *query, *topSeeds)
	if len(seeds) == 0 {
		fmt.Println("No sections matched this query at all.")
		return
	}

	seedIDs := make([]string, len(seeds))
	for i, s := range seeds {
		seedIDs[i] = s.SectionID
	}

	fmt.Printf("Seed section(s): %v (hops=%d)\n\n", seedIDs, *hops)
	expansion := g.Expand(seedIDs, *hops)

	fmt.Println("--- Sections included in context ---")
	for _, s := range expansion.Sections {
		fmt.Printf("  %s: %s\n", s.ID, s.Title)
	}

	answer := synth.Synthesize(*query, expansion)
	fmt.Println("\n--- Answer ---")
	fmt.Println(answer.Text)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
