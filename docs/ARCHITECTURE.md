# Architecture

## What problem this solves, and why it's architecturally distinct from this author's existing Hybrid RAG

Chunk-based retrieval (including hybrid lexical+vector retrieval) answers
"what text is most similar to this question." That's the wrong question
for a large class of real legal/regulatory queries, because the correct
answer often lives in a DIFFERENT section than the one the question's
words most resemble — an exception clause, an amendment, a definition —
connected to the matched section by a citation relationship, not by
textual similarity. This project is built around that specific failure
mode: retrieval that follows a citation GRAPH outward from a lexically-
matched seed, rather than retrieval that only ever returns more of what
already looked similar. This is the structural difference from this
author's existing Hybrid RAG project, which (like virtually all chunk-
based RAG, hybrid or not) has no mechanism to reach a chunk that doesn't
resemble the query at all but is nonetheless required to answer it
correctly.

## The capstone proof: a query where this difference is not theoretical

`test/integration/multihop_test.go`'s
`TestHealthcareExceptionRequiresMultiHop` is built so the failure mode is
concrete, not asserted in the abstract: the bundled corpus's healthcare
exception (`s9`, "Sensitive Data... may be retained for up to 7 years")
is a WORSE lexical match for a question about hospital retention periods
than the general rule (`s1`, "90 days") is — which is realistic: exception
clauses often use more specific, different vocabulary than the question
asking about them. A pure top-k chunk retriever handing a synthesizer only
the top lexical match would produce a confident, complete-looking, and
flatly wrong "90 days" answer for a hospital's patient records. One hop of
graph traversal along the `exception_to` edge reaches the actual 7-year
answer; the test asserts the 0-hop context provably lacks it (not just
that the 1-hop context has it) — so the comparison is "wrong vs. right,"
not "less detailed vs. more detailed."

## Two distinct inclusion mechanisms, not one

`internal/graph.Expand` assembles context two different ways, deliberately
kept separate:

- **Bounded-hop BFS** over the citation graph (`references`, `amends`,
  `exception_to` edges), stopping at a configurable hop limit. This is
  "what's connected to this section, within N steps."
- **Definition pull-in**, which ignores hop distance entirely: whenever
  any collected section's text uses a term the corpus defines elsewhere,
  that definition is included regardless of how far away it is in the
  citation graph — run to a fixed point, so a definition that itself uses
  another defined term pulls that one in too
  (`TestDefinitionPullInIsTransitive`). A term definition isn't "one hop
  of relatedness" away from a section that uses it; it's necessary
  context for understanding that section's text at all, which is a
  categorically different relationship than a citation.

## A real bug found via the test suite: traversal must be bidirectional

The amendment-history scenario (`TestAmendmentHistoryRequiresFollowingAmendsEdge`)
seeds on the CURRENT rule (Section 3, 72-hour breach notification) and
asks about its history. The corpus models this realistically: the 2015
amendment section has an outgoing edge referencing both the 1998 original
AND Section 3 (because an amendment naturally cites what it changed and
what it changed it to) — but Section 3 itself has no outgoing edge to
either of those, since it predates the amendment and has no reason to
reference forward to its own future history.

The first version of `Expand` only followed a section's own outgoing
`Refs`, which is forward-only graph traversal. Running the test revealed
this immediately: seeded on Section 3, forward-only BFS could never reach
the amendment section or the original rule, no matter the hop count,
because the only edges that *exist* point the other way. Fixed by making
`Graph.Build` construct a bidirectional adjacency list — every reference
creates a traversal edge in both directions — while `Reference.Kind`
itself remains directional and semantically meaningful for
`internal/synth`'s explanatory phrasing ("X amends Y," not "X and Y are
mutually related"). `TestExpandFollowsReferencesBackwardNotJustForward`
is the regression test, isolated at the unit level from the integration
test that first surfaced the gap.

## Explicit reference markup, not NLP-extracted citations

Real citation extraction from legal prose — resolving "as provided in
subsection (b) above" or "the preceding Article" into a concrete section
ID — is a serious, separate natural-language problem. This project's
corpus (`internal/document`) uses explicit `refs: [{target, kind}]`
markup instead, which is a deliberate scope decision: it keeps the
corpus's graph shape exactly known and testable (every test above can
assert on precise hop counts and exact node sets), so the engineering
effort stays on graph construction, bidirectional traversal, and
definition pull-in — not on the separate, much larger problem of citation
extraction from unstructured text. `document.Load` does validate that
every reference resolves to a real section and a known relation kind, so
a corpus authoring mistake is caught immediately rather than silently
producing a graph with the wrong shape.

## Testing strategy

- `internal/document`: corpus validation — duplicate IDs, dangling
  references, unknown relation kinds.
- `internal/graph`: hop-limit boundaries (0, 1, 2 hops), the cycle-safety
  test (circular references — a real, plausible shape in legal text —
  must not hang or loop forever), no-duplicate-node-via-two-paths,
  definition pull-in at zero hop distance and transitively through a
  chain of definitions, whole-word (not substring) term matching, and the
  bidirectional-traversal regression test described above.
- `internal/retrieval`: lexical ranking and topN behavior.
- `internal/synth`: that an exception present in context is explicitly
  surfaced, and — the critical negative case — that an exception NOT in
  context is never mentioned, proving an incomplete context produces a
  genuinely incomplete answer rather than one that "knows" things it
  wasn't given.
- `test/integration`: the two capstone scenarios (healthcare exception,
  amendment history), each asserting the exact section-set difference
  between hop depths against a real, loaded corpus file — not synthetic
  data constructed just for the test.

## What's deliberately out of scope (v1)

- **NLP-based citation extraction from unstructured legal prose.** See
  above — explicit markup is the chosen scope, with the engineering
  effort concentrated on graph construction and traversal instead.
- **Edge-type-filtered traversal** (e.g., only following `exception_to`
  edges for a question specifically about exceptions). `Expand` currently
  traverses all edge kinds uniformly; `Reference.Kind` is preserved and
  surfaced in explanations, but doesn't yet gate which edges a particular
  query follows. A reasonable v2 extension once a real query-classification
  signal exists to drive it.
- **A real embedding-based or LLM-judged seed retriever.** Lexical
  word-overlap is enough to prove the point this project is actually
  about (what happens AFTER a seed is found); swapping in a smarter
  retriever wouldn't change any of the graph logic this project tests.
