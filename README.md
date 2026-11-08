# graphrag-legal

GraphRAG for legal/regulatory documents: a citation graph over document
sections (references, amendments, exceptions, definitions) with bounded-
hop traversal and transitive definition pull-in — built specifically to
answer the question chunk-based top-k retrieval structurally cannot:
"what else, possibly nothing like this question's wording, do I need to
read to answer this correctly."

```
query ──▶ lexical seed-finding ──▶ seed section(s)
                                         │
                            bounded-hop BFS (bidirectional,
                            cycle-safe) over references/
                            amends/exception_to edges
                                         │
                      + transitive definition pull-in
                      (any defined term used anywhere
                       in the collected text, regardless
                       of hop distance)
                                         │
                                 synthesized answer
```

## Why this project, and why it's a different architecture from this author's Hybrid RAG

A chunk-based retriever (lexical, vector, or hybrid) only ever returns
text that resembles the query. A huge class of real legal questions needs
information that *doesn't* resemble the query at all — an exception
clause phrased in entirely different vocabulary, an amendment that
changed what a section used to say, a definition two citation-hops away
that's necessary to even parse the matched section correctly. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full reasoning,
including a real bug this project's own test suite found: the first
version of graph traversal only followed a section's own outgoing
references, which cannot answer "what's this rule's amendment history"
when seeded on the current rule — the amendment section references
backward to what it changed, not the other way around. Fixed by making
traversal bidirectional, with a dedicated regression test.

## Quickstart

```bash
make build
make test        # 25+ tests, no external services
make demo         # the same query at 0 hops vs 1 hop, side by side
```

```bash
./bin/graphrag -query "What is the retention period for hospital patient records?" -hops 0
./bin/graphrag -query "What is the retention period for hospital patient records?" -hops 1
```

At 0 hops (what chunk-based top-k retrieval would give you), the answer
is a confident, complete-looking, **wrong** "90 days." At 1 hop, the
answer correctly surfaces the healthcare exception's 7-year period — plus
the two term definitions needed to understand why it applies — because
graph traversal, not lexical similarity, is what reaches them.

## What's actually proven, concretely

- **A query whose correct answer requires multi-hop traversal, proven as
  wrong-vs-right, not merely less-detailed-vs-more-detailed**: the
  exception clause is a WORSE lexical match for the query than the
  general rule it overrides — realistic, since exception clauses use
  different vocabulary than the question asking about them — so a pure
  top-k retriever would confidently return the wrong answer.
- **Cycle-safe traversal**: circular references (a real, plausible shape
  in legal text — a rule and its exception referencing each other) don't
  hang or infinite-loop.
- **Transitive definition pull-in**: a definition that itself uses another
  defined term correctly pulls that one in too, run to a fixed point —
  and whole-word term matching, so a defined term like "Data" doesn't
  spuriously match inside "Database."
- **Bidirectional traversal, found necessary via a real test failure**: an
  amendment section naturally references backward to what it amended;
  understanding a current rule's history requires walking that same edge
  backward from the current section, which a forward-only traversal
  structurally cannot do.

## Project layout

```
internal/document/    corpus format (sections + typed references + defined terms) + validation
internal/graph/         citation graph construction, bidirectional bounded-hop BFS, definition pull-in
internal/retrieval/      lexical seed-finding
internal/synth/          answer assembly from the expanded graph context, with explicit exception/amendment surfacing
cmd/graphrag/            CLI: ask a query at a configurable hop depth
testdata/docs/corpus.yaml  the bundled fictional "Data Protection Act" corpus
test/integration/         the two capstone wrong-vs-right multi-hop scenarios
docs/ARCHITECTURE.md    design rationale, including the bidirectional-traversal bug found and fixed
```

## What's deliberately out of scope (v1)

See docs/ARCHITECTURE.md — no NLP-based citation extraction from
unstructured prose (explicit reference markup is the chosen scope), no
edge-type-filtered traversal, no embedding-based retriever (lexical
seed-finding is enough to prove the point this project is actually about).
