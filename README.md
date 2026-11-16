# graphrag-legal

A small, from-scratch **GraphRAG** engine for legal and regulatory text — retrieval-augmented generation (answering questions using real documents, instead of letting a model guess from what it already "knows") where the documents aren't just a pile of independent chunks, but a **citation graph**: a network of sections connected by *references* ("see Section X"), *amendments* ("this section changes what Section Y used to say"), *exceptions* ("this section carves out a special case to Section Z's rule"), and *definitions* ("this section is the authoritative meaning of the term 'Sensitive Data'"). Answering a question means finding the best initial textual match and then **walking that graph outward from it**, rather than stopping at whichever single chunk of text happens to share the most words with the question.

This is project 3 of 3 in a hiring portfolio, and it is deliberately a different architecture from the author's existing chunk-based Hybrid RAG project. That project retrieves the top-k most similar chunks (by lexical search, vector search, or both) and hands them to a model. This project exists because, for a large and important class of legal/regulatory questions, "most similar chunk" and "actually correct answer" are different questions — and no amount of better embeddings or better hybrid ranking closes that gap, because the gap isn't about *similarity quality*, it's about the correct answer living somewhere the question's wording never points to at all.

## Why this exists, and how it's different from a normal RAG system

A normal retrieval system — lexical keyword search, vector/embedding search, or a hybrid of the two — works by comparing the words (or meaning) of a question to the words (or meaning) of every chunk of text in a document collection, and returning whichever chunks score highest. This works well when the answer is phrased similarly to the question. It fails, silently and confidently, when the answer is phrased *differently* from the question — which is the normal case in legal and regulatory writing.

Legal text is full of sections that only make sense in relation to other sections. A general rule says one thing; a separate, specifically-worded exception two pages away overrides it for a narrow case. A rule as written today says one thing; an amendment section, written years later in different words, explains that it used to say something else. A sentence uses a capitalized term like "Sensitive Data" as if the reader already knows exactly what that means, and the actual definition lives in a completely different section. None of these connections are visible from word-overlap or semantic similarity alone — they're visible only if you know, structurally, that section A is linked to section B as "the exception to A," or "the amendment of B," or "the definition of a word B uses."

That's the architectural difference this project is built around: instead of only ever returning *more text that resembles the query*, it builds a graph of how the document's sections relate to each other, finds a starting point the normal way (textual similarity), and then follows the graph's edges outward from there to pull in whatever else is actually needed — even text that doesn't resemble the question's wording even slightly.

## The core example

The bundled demo corpus (`testdata/docs/corpus.yaml`) is a small fictional "Data Protection Act" built specifically to make this failure mode concrete, not abstract. It contains:

- **Section 1** — the general rule: personal data must be deleted after **90 days**.
- **Section 9** — the healthcare exception: "Sensitive Data" processed by a healthcare provider for treatment may be retained for **7 years**, "notwithstanding the general retention period" in Section 1. Section 9 is linked to Section 1 with an `exception_to` relationship.
- Two definition sections spelling out what "Sensitive Data" and "Biometric Identifier" actually mean.

Now ask: **"What is the retention period for hospital patient records?"**

A chunk-based retriever — lexical, vector, or hybrid — scores every section against that question's wording and picks the best match. Section 1 ("retention period," "90 days," "personal data") shares far more vocabulary with the question than Section 9 does, because Section 9 is written in the specific, different language of a carve-out ("Notwithstanding... Sensitive Data... healthcare provider... treatment purposes"), not in the question's own words. So a pure top-k retriever hands a generator *only* Section 1, which produces a confident, complete-looking, **and flatly wrong** answer: "90 days." Nothing about that answer looks incomplete — it reads like a full, correct response, which is exactly what makes this failure mode dangerous: there's no obvious seam where a reader would think to double-check it.

This project proves the alternative isn't just a theory. `test/integration/multihop_test.go`'s `TestHealthcareExceptionRequiresMultiHop` runs this exact question through the pipeline and asserts, as a hard test — not a narrated claim — that: (1) Section 1 really is the single strongest lexical match (so the test isn't rigged in GraphRAG's favor), (2) the 0-hop answer (what a chunk-based retriever would produce) states "90 days" and provably does **not** contain "7 years" anywhere in its context, and (3) the 1-hop answer — found by walking one step across the `exception_to` edge from Section 1 to Section 9 — correctly surfaces the 7-year exception, *plus* the definition of "Sensitive Data" needed to understand why the exception applies at all. The comparison is wrong-vs-right, not merely shallow-vs-detailed.

## How it works

A query flows through four stages:

**1. Find the best initial textual match (seeding).** `internal/retrieval` scores every section in the corpus by simple word overlap with the question (after stripping common stop words like "the," "is," "of") and returns the top-scoring section(s) as seeds. This step is deliberately simple lexical matching — the point of this project is what happens *after* a seed is found, not how sophisticated the initial match is. A smarter retriever (embeddings, an LLM judge) could be swapped in here without touching anything described below.

**2. Walk the citation graph outward from the seed, up to a hop limit.** `internal/graph` builds an adjacency list from every section's typed references (`references`, `amends`, `exception_to`) and performs a breadth-first search starting at the seed(s), stopping once it has gone `-hops N` steps away. A hop count of 0 means "just the seed, nothing more" — which is effectively what a chunk-based retriever gives you. A hop count of 1 or 2 follows the actual citation links outward. Because legal text sometimes has circular references (a rule and its exception can reference each other back and forth), the traversal tracks which sections it has already visited and never re-queues one, so a cycle can't cause it to loop forever.

**3. Separately pull in any definitions needed to understand what's been collected.** This is a *second, deliberately distinct* mechanism from hop-counting. After the bounded-hop walk finishes, the system scans every piece of text it has collected so far for any term the corpus defines elsewhere (matched as a whole word, so "Data" doesn't spuriously match inside "Database"). If a term is found, that term's definition section is pulled in — **no matter how many hops away it is**, and with no hop budget charged against it. If that newly pulled-in definition *itself* uses another defined term, that one is pulled in too, and this repeats until a full pass adds nothing new. The reasoning for treating this differently from hop-counting: knowing what a defined term means isn't "one step of relatedness" away from the text that uses it — it's required to understand that text *at all*. Bounding it by hop distance would be like understanding 90% of a sentence and guessing at the rest.

**4. Assemble an answer, calling out exceptions and amendments explicitly.** `internal/synth` builds the final answer from exactly the sections the previous steps assembled — nothing more. If any `exception_to` or `amends` relationship is present in that context, it's surfaced explicitly in the answer's text ("However, Section 9 establishes an exception: ...") rather than left for the reader to notice on their own, because a legal answer that silently omits "but there's an override for your case" is exactly the kind of confidently-wrong answer this project is built to avoid. The synthesizer never knows anything the graph expansion didn't hand it — so a wrong or incomplete answer is always traceable to a gap in what got retrieved, not to the write-up stage.

## Architecture at a glance

| Package | Responsibility |
|---|---|
| `internal/document` | The corpus format — sections, each with a title, body text, a list of typed outgoing references (`target`, `kind`), and optionally a list of terms it defines. `Load` validates the corpus: every reference must point at a section that actually exists, and every relation kind must be one of the known ones, so a corpus-authoring mistake is caught immediately instead of silently producing a graph with the wrong shape. |
| `internal/graph` | Builds the citation graph from loaded sections and implements `Expand`: the bounded-hop, cycle-safe, bidirectional breadth-first search, plus the separate transitive definition pull-in described above. |
| `internal/retrieval` | Lexical seed-finding — scores sections by word overlap with the query and returns the top N. |
| `internal/synth` | Turns an assembled graph context into a final answer, explicitly surfacing any exception or amendment relationship present in that context. |
| `cmd/graphrag` | The CLI entry point: load a corpus, find seeds, expand the graph to a configurable hop depth, synthesize and print the answer. |
| `testdata/docs/corpus.yaml` | The bundled fictional "Data Protection Act" — the general retention rule, the healthcare exception, a breach-notification rule with its own amendment history, and two definition sections. |
| `test/integration` | The two capstone end-to-end scenarios: the healthcare exception (above) and the amendment-history case (below), each run against the real loaded corpus file. |
| `docs/ARCHITECTURE.md` | The full design rationale, including the bug story below in more detail. |

## A real bug our own tests caught

This project's test suite didn't just confirm the design worked — it found a real structural gap in the first version of graph traversal, and the fix is one of the more interesting pieces of engineering in this codebase.

The first version of `Graph.Expand` followed only a section's own **outgoing** references: if section A's `refs` list pointed at section B, traversal could go from A to B, but not from B back to A unless B *also* happened to list a reference back to A.

That's a reasonable first instinct, and it's wrong for a scenario that's completely ordinary in real regulatory text: **amendment history**. The bundled corpus models a breach-notification rule with a real history — a 1998 original section required notification within 30 days; a 2015 amendment section shortened that to 72 hours, and that 72-hour rule is what the current Section 3 states. The 2015 amendment section naturally has an outgoing reference to the 1998 section it amends (`amends: s4-original-breach-window`) — an amendment, almost by definition, has to reference what it's changing. But the current rule, Section 3, has **no outgoing reference to its own amendment or to the 1998 original** — and there's no reason it should, since Section 3 predates the 2015 amendment section and the 1998 original alike. It can't reference its own future history before that history exists.

So picture the realistic query: *"How quickly must a company notify people after a breach, and has this deadline changed over time?"* That question's best lexical match is Section 3 (the current 72-hour rule) — and from Section 3, forward-only traversal has **no edge to follow at all** toward the amendment or the original, no matter how many hops you allow, because the only edges that connect Section 3 to its own history point the *other* direction. A test built around exactly this scenario (`TestAmendmentHistoryRequiresFollowingAmendsEdge`) was written and failed immediately on the first implementation — not because of a typo or an off-by-one, but because the traversal was structurally incapable of reaching the answer from that starting point.

The fix: `Graph.Build` now constructs its traversal adjacency list **bidirectionally** — every reference, in either direction, creates a connection usable for traversal in both directions — while `Reference.Kind` (`references`, `amends`, `exception_to`) is preserved exactly as written and stays fully directional for explanation purposes, so `internal/synth` can still correctly say "Section 5 amends Section 4," never the meaningless "Section 4 and Section 5 are mutually related." Traversal connectivity became undirected; the semantic meaning of each link did not. A dedicated, isolated unit test, `TestExpandFollowsReferencesBackwardNotJustForward`, was added afterward specifically so this exact mistake can never silently come back, separate from the integration test that first surfaced the gap.

## Try it yourself

```bash
make build
make test   # 25 tests, no external services, no network calls
make demo   # the same query run at 0 hops and 1 hop, side by side
```

`make demo` (or `scripts/run-demo.sh`) runs the exact healthcare-retention question through the CLI twice — once at `-hops 0` and once at `-hops 1` — so you can watch the wrong-then-right difference happen directly in your terminal, not just read about it in a test assertion:

```bash
./bin/graphrag -query "What is the retention period for hospital patient records?" -hops 0
./bin/graphrag -query "What is the retention period for hospital patient records?" -hops 1
```

At 0 hops, the output states the general 90-day rule and stops there — a confident, complete-looking, wrong answer. At 1 hop, the output additionally surfaces Section 9's 7-year healthcare exception and the "Sensitive Data" definition needed to understand it, because the graph walk — not lexical similarity — is what reaches them. You can also point `-corpus` at your own YAML file, or raise `-hops`/`-seeds` to explore further.

## What this deliberately doesn't do (yet)

- **No NLP-based citation extraction from unstructured prose.** Real legal documents don't write `refs: [{target: s9, kind: exception_to}]` — they write "Notwithstanding the general retention period in the preceding section." Automatically resolving phrases like that (or "as provided in subsection (b) above," or "the preceding Article") into a concrete target section is a serious, separate natural-language problem in its own right. This project's corpus uses **explicit, typed reference markup** instead, as a deliberate scope choice: it keeps the graph's shape exactly known and testable (every test in this repo can assert on precise hop counts and exact section sets), which let the actual engineering effort go into the graph-walking logic itself — bidirectional traversal, cycle safety, transitive definition pull-in — rather than into citation extraction.
- **No edge-type-filtered traversal.** `Expand` currently walks all reference kinds uniformly; it doesn't yet, say, follow only `exception_to` edges for a question specifically about exceptions. `Reference.Kind` is preserved and used for explanation, but doesn't gate traversal yet — a reasonable extension once there's a real signal for classifying what kind of question is being asked.
- **No embedding-based or LLM-judged seed retriever.** Seeding uses plain lexical word-overlap. That's enough to prove the point this project is actually about — what happens to the context *after* a seed is found — and swapping in a smarter retriever wouldn't change any of the graph-traversal logic this project tests.
- **No real language-model generation.** `internal/synth` assembles an answer by concatenating the collected sections with explanatory framing, not by calling an LLM. This keeps the correctness story isolated to retrieval: a wrong or incomplete answer here is always traceable to what did or didn't make it into the context, never to a generation quirk.

## Project layout

```
cmd/graphrag/              CLI: ask a query at a configurable hop depth
internal/document/         corpus format (sections + typed references + defined terms) + validation
internal/graph/             citation graph construction, bidirectional bounded-hop BFS, definition pull-in
internal/retrieval/          lexical seed-finding
internal/synth/               answer assembly, with explicit exception/amendment surfacing
testdata/docs/corpus.yaml  the bundled fictional "Data Protection Act" demo corpus
test/integration/          the two capstone wrong-vs-right multi-hop scenarios
scripts/run-demo.sh        the 0-hop-vs-1-hop side-by-side CLI demo
docs/ARCHITECTURE.md       full design rationale, including the bidirectional-traversal bug in more detail
```

## License

[MIT](LICENSE) — see the LICENSE file for the full text.

Source: [github.com/Kuwar-Vishnudatt/graphrag-legal](https://github.com/Kuwar-Vishnudatt/graphrag-legal)
