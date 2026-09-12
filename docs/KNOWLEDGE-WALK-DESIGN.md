# Knowledge Walk mode — design notes

Status: prototyped and running locally (`cmd_leanwalk.go`, `lean-walk.html`,
`liuk leanwalk`/`liuk leanwalk-relabel`) - see "Lean walk, take two" below for
the current, corrected version. Numbers below are measured against the
`graph-data` embedded in `knowledge-map.html` as of 2026-09-12 and will drift
as `source-data.json` changes — re-run the checks in "How this was verified"
before relying on exact figures again.

## The pitch

A mode built on top of the existing Knowledge Map: pick a cluster of the
graph, then answer its questions one at a time as a walk — every next
question is guaranteed to share, or be one hop from, an entity in the
question before it. Never a non-sequitur jump to an unrelated topic.

## Data model (recap, not new)

`knowledge-map.html` embeds one JSON blob with:

- `nodes`: entities (person/place/event), each with the question IDs that
  mention it.
- `edges`: entity–entity co-occurrence (two entities share an edge if some
  question mentions both), with the list of questions that created the edge.
- `questions`: the full question bank, keyed by ID.

Current shape: 428 total questions. 106 have zero tagged entities, 99 have
exactly one (no edges), 322 have ≥1 entity, forming 340 entity nodes / 1116
edges. **The entity graph is not connected** — it splits into 33 components
("islands"): one giant island (266 entities, covering 258 questions), 12
small-but-real islands (2–11 entities each), and 20 fully isolated
single-entity islands.

## How we got here (the reasoning, compressed)

1. **"Walk to adjacent nodes only" ≠ Hamiltonian path.** A walk that's
   allowed to revisit nodes (plain DFS with backtracking) trivially covers
   every node in one connected component — no NP-hardness involved. The real
   limiter is that the graph is fragmented into 33 islands; you can never
   cross between islands via adjacency.

2. **Does splitting into islands make a true no-repeat tour (a Hamiltonian
   path) possible per island?** Checked exhaustively, not assumed:
   - All 32 non-giant islands: **yes**, confirmed by brute-force search
     (they're small enough to search completely).
   - The giant island: **provably no.** It has 10 entities of degree 1 (each
     co-occurs with exactly one other entity — e.g. Adam Smith, David Hume,
     Great Famine (Ireland)). A Hamiltonian path has only 2 endpoints; a
     degree-1 node can only ever be an endpoint (it has just one edge to
     spend); 10 forced endpoints is a contradiction. No search needed to know
     this one's impossible.

3. **Reframe: a node is a topic with a queue of questions, not one
   question.** So `A(q1) → Great Famine → A(q2)` is a legitimate, honest
   transition — A and Great Famine really are adjacent, and A just hands out
   its second question on the second visit. Simulated a plain DFS on the
   giant island under this model: 531 node arrivals, 243 surface a fresh
   question, 288 are free "silent" hops (the topic's queue is already empty).
   No transition is faked — unlike silently hiding a repeated node, which we
   also checked and rejected (it would fake non-adjacent transitions on
   roughly half the walk; see git history of this conversation for the
   numbers if needed).

4. **Reframe again: build the graph on *questions*, not entities.** Connect
   question *i* and *j* if they share a tagged entity, or their entities are
   adjacent in the original graph (a "vertex blow-up" / clique-expansion
   construction). This erases the killer leaf constraint from step 2: a
   lonely leaf entity's one question inherits every one of its neighbor's
   *other* questions as new connections, so it stops being degree-1. Verified
   on the giant island's question graph (258 questions): **zero degree-1
   questions** (min degree 2, median 58 — it gets dense fast). A plain greedy
   heuristic (Warnsdorff's rule: always step to the unvisited neighbor with
   the fewest remaining options — the knight's-tour trick) found a **complete
   258/258 Hamiltonian path on the first try**, and succeeded from 257 of the
   258 possible starting questions tried individually.

5. **Hardcode the discovered path, or compute at runtime?** Neither, quite —
   landed on build-time generation of a *pool*. Reasoning:
   - `knowledge-map.html` currently has **no regeneration pipeline** at all
     (unlike `exams.json`/`exams.md`, which `cmd_generate.go` rebuilds from
     `source-data.json`) — it's a static, hand-committed snapshot. A single
     baked path risks silent staleness the moment source data changes, with
     no way to notice.
   - A single fixed path also can't honor "pick your own start" — a
     Hamiltonian path has only two true starting points.
   - Pure runtime discovery in the browser works, but means any greedy-search
     failure becomes *a live user's* stuck walk, discovered by them, with no
     record anyone can act on.
   - Best of both: generate a **pool** of complete traversals as part of the
     Go build/generate step (extending `cmd_generate.go` and, first, actually
     giving `knowledge-map.html` a regeneration path — currently missing).
     Verification then happens at build time, and can fail loudly the same
     way `cmd_generate.go` already warns about uncategorized questions,
     instead of silently breaking for a real user.

6. **Storage cost of a pool (measured, not guessed):**
   - Traversal as raw 10-char hex question IDs: 3.6KB raw / 1.8KB gzip.
   - Traversal as indices into one shared per-island lookup table (stored
     once): 923B raw / 458B gzip standalone, dropping to **~220B/traversal**
     when many are gzipped together as a batch — close to the true
     information floor (~213B for an arbitrary 258-permutation, ~143B for one
     constrained to real graph edges).
   - At ~220B/traversal, **2,000 traversals for the giant island ≈ 44KB
     gzip** — trivial next to the existing 368KB `knowledge-map.html`.
   - Sizing rule of thumb, since people drill these questions repeatedly and
     *will* notice a repeat eventually: expect the first repeat around √N
     plays (birthday paradox) — N=2000 → first repeat around play ~45. Cost
     is flat enough in this range to just round up rather than tune tightly.

7. **Naive "loop until N successes" has two real failure modes** (found by
   testing, not assumed):
   - Some individual starting questions are **permanently, structurally
     unsolvable** by the greedy heuristic — not unlucky: one giant-island
     start failed 0/200 across completely different random tie-break seeds.
     If a fixed start happens to land there, `while successes < N` never
     terminates.
   - Small islands **exhaust their pool of distinct orderings** long before
     N — a 2-question island has exactly one possible ordering.
   - Fix: don't pre-validate one fixed start. Pick a **random start on every
     attempt** across the whole island, run the greedy heuristic once, keep
     it if complete, discard if not — no special-casing needed. Cap total
     *attempts* (e.g. 20×N or a time budget), separately from the success
     target N, and warn rather than hang if the cap is hit first.

8. **That naive random-restart loop also satisfies the final UX target for
   free** (tested against the giant island):
   - 2,007 attempts produced all 2,000 target successes — only 7 wasted.
   - 257 of 258 possible starting questions ended up represented in the pool.
   - Reaching **≥4 distinct successful starting nodes** (the actual bar,
     since the UX only ever needs to offer a 4-way choice) took **4
     attempts**.
   - Success counts were fairly even across starts — no single one dominated
     the pool, so the "which 4 do we offer" choice can rotate across visits.

## Current design

- **Product shape:** "Pick your island" (one of the 33 connected components)
  → app offers a 4-way multiple choice of distinct starting nodes, each
  backed by one verified, complete, zero-repeat traversal → the chosen
  traversal plays back as a fixed sequence of questions, each genuinely
  adjacent to the last.
- **Generation (build time, in Go):**
  1. Give `knowledge-map.html`'s `graph-data` an actual regeneration path
     from `source-data.json`/`categories.json` (it has none today — this is
     a prerequisite, not optional, or the pool below bakes in the same
     staleness risk it's meant to avoid).
  2. For each of the 33 islands, build the question-level graph (same-entity
     or entity-adjacent) and run the randomized-restart-and-discard loop:
     random start each attempt, keep complete traversals, discard
     incomplete ones, stop at N successes or an attempt cap (whichever
     first), warn if the cap is hit short of N.
  3. Encode each traversal as indices into a per-island lookup table of
     question IDs; dump the pool alongside the graph data.
- **Runtime:** no search or traversal logic ships to the client at all —
  just pick 4 distinct-start traversals from the relevant island's pool to
  offer, then replay whichever one the user picks.

## A second mode: "lean walk" (minimize revisits instead of maximizing coverage)

Everything above ("completionist") treats 100% question coverage as the
goal, which by definition forces revisiting hub entities as many times as
they have questions (London has 14 questions — all 14 have to surface
*somewhere*, however you order things). A different, equally valid target:
**touch every entity at least once, then revisit as few times after that as
possible**, accepting that some hub-heavy questions never come up in a given
walk.

- **Theoretical floor:** same leaf-counting logic as the Hamiltonian-path
  proof, flipped. The giant island's 10 degree-1 entities each force one
  dead-end-and-return unless they're the walk's start or end (only 2 free
  passes exist) → **at least 8 revisits are unavoidable**, no matter the
  ordering.
- **What's actually achievable:** a corrected DFS (only backtrack when a
  node has genuine unfinished business, not unconditionally after every
  branch — an earlier version of this check had a bug that made revisit
  count look identical regardless of ordering; fixed) gets to **~51–65
  total revisits** across all 266 entities, surfacing **~202–213 of the 258
  possible questions (~78–83%)**. Closing the remaining gap to the 8-revisit
  floor is itself the same flavor of NP-hard problem as the Hamiltonian path
  question (minimum-leaf spanning tree) — not chased to an exact optimum,
  just confirmed the achievable ballpark.
- **This is a genuinely different mode from completionist, not a tuning
  knob on it** — same island, same graph, deliberately different tradeoff
  (breadth over completeness).

### Does the "no immediate ping-pong" rule apply here too?

Raised because a completionist walk can produce `A → B → A → B → A → B → A`
between two question-rich, mutually-adjacent entities — technically valid
(every hop is a real adjacency), but tedious. Checked both modes:

- **Completionist (question-graph) mode:** the naive greedy search *wants*
  to oscillate, because every return to a rich hub yields a fresh unseen
  question — that's a real reward under a "maximize coverage" objective.
  Added a constraint (forbid stepping to a question anchored back to the
  same entity as two steps ago, i.e. a topic-level non-backtracking walk):
  confirmed compatible — complete 258/258 solutions satisfying it exist —
  but **success rate per search attempt drops ~200x** (from ~100% to
  ~0.5%), meaning generating the same size pool costs roughly 200x more
  search attempts (still just single-digit minutes in Go, given how fast
  each individual attempt is — just not the "basically free" story the
  unconstrained pool generation had). A third of failed attempts land at
  257/258 (a near miss) rather than collapsing, so a light backtrack-and-retry
  instead of blind restart-on-failure would likely recover most of that
  200x tax cheaply — not implemented, just identified as the obvious next
  optimization if this constraint is kept.
- **Lean (entity-level, minimize-revisits) mode: the constraint is already
  satisfied for free.** Checked 10 different lean-walk runs: **zero cases,
  in any run, of the same (A, B) pair round-tripping more than once.**
  Reasoning: a lean walk only revisits a node when the graph structurally
  forces it (dead end, or returning to reach a different branch), and once
  revisited there's nothing left to gain from a third visit — no reward
  structure exists to make oscillation attractive the way it does under a
  coverage-maximizing objective. The 10 forced degree-1 leaf round-trips are
  each single out-and-backs (visit leaf, forced return, never repeated),
  not the repeated-oscillation pattern the rule exists to prevent. So: no
  extra constraint code needed for lean walk — implementing the lean search
  already implies it.

## Lean walk, take two: the entity-DFS version under-delivered, replaced

The first lean-walk implementation (entity-level DFS, minimize revisits -
everything in the section above) shipped as a prototype (`cmd_leanwalk.go`,
`lean-walk.html`) and got real play-testing, which surfaced a problem the
design work above never caught:

### The "via" label was frequently disconnected from what the player just read

Asked directly by the player: given `Q1 (tagged: Margaret Thatcher, Harold
Wilson, James Callaghan, Tony Blair, John Major) → Q2 (tagged: Mary Queen of
Scots, Queen Victoria, House of Lancaster, Henry VII, Wars of the Roses,
Elizabeth of York, Queen Anne)`, shown as "connected via Mary, Queen of
Scots" - how does Q1 connect to that at all? Traced it: Margaret Thatcher and
Mary, Queen of Scots are genuinely adjacent in the entity graph (they
co-occur in a *third* question the player never saw), so the strict-adjacency
guarantee held - but the label only ever showed the *current* question's
entity, never the *previous* question's entity that was actually doing the
bridging. Measured across the pool: **this wasn't rare - 80.8% of all
step-transitions were this "adjacency-only" kind** (only 19.2% were a
directly shared entity), because the search's own objective (maximize new
entity coverage per step) actively prefers jumping via loose adjacency over
sharing an entity, since sharing "wastes" coverage overlap.

### Is the adjacency mechanism even a good abstraction, then?

Player's follow-up, more fundamental: since "adjacent to Thatcher" really
means "co-occurred with Thatcher in *some* question, not necessarily this
one," restrict the walk to only jump between entities *actually tagged on
Q1 itself* (i.e. Thatcher/Wilson/Callaghan/Blair/Major) - drop the looser
adjacency clause entirely. Tested rather than assumed:

- **Dropping it outright makes full coverage provably impossible** - the
  same leaf-count proof from the entity-Hamiltonian-path question earlier in
  this doc recurs, just relocated: 10 questions end up with degree ≤1 in the
  shared-entity-only question graph (their tagged entities individually
  appear in only 1-2 questions total, e.g. a question tagged only with
  "Great Depression" (count 2) has exactly one other question to connect
  to). A no-repeat path can have only 2 such endpoints; here there are at
  least 10. 3,300 search attempts confirmed it empirically: 0 successes.
- **Even just *softening* it** (prefer shared-entity moves when available,
  fall back to adjacency only when no shared option exists) broke the
  search too: 0/2,500 attempts succeeded, median coverage stalling at
  155/266. Plain greedy has no foresight that it'll need the adjacency
  escape hatch later, so biasing away from it early strands the walk.
  Concluded: the adjacency clause is load-bearing for feasibility, not an
  optional flourish, and biasing the *search* against it isn't a cheap fix -
  it would need real backtracking/lookahead, a materially bigger lift than
  anything built so far.

### The fix that actually worked: cite the evidence instead of hiding it

The entity graph already records, per edge, which literal question
established the co-occurrence (it's how the edge was built in the first
place). Rather than fight the search, surface that citation directly: when a
step's connection is adjacency-only, show both ends of the bridge *and* the
actual third question, e.g. "connected via **Margaret Thatcher → Mary,
Queen of Scots** — both appear together in: 'Who was the first female Prime
Minister of the UK?'". This turns an unfalsifiable-feeling assertion into a
checkable fact, with no change to the search and no cost to feasibility.
Confirmed 100% of adjacency-only steps in the current pool now carry a
citation.

### The corrected design (what's actually running now)

- **Search moved from entity-level DFS to the question graph** (same
  "blow-up" construction completionist mode uses: nodes = questions, edges
  = shared-or-adjacent entity), greedily maximizing new-entity coverage per
  step, stopping the instant all 266 entities are covered. This is strictly
  better than the DFS version: every step is now genuinely, strictly
  adjacent to the last (no silent skips at all - the DFS version's silent
  hops meant ~16% of *shown* question pairs weren't actually adjacent to
  each other, only connected through a skipped middle entity), and walks
  got *shorter*, not longer, because a single multi-entity question can
  satisfy several entities' coverage at once - something entity-by-entity
  DFS had no way to exploit.
- **Generation now runs the full 5-minute budget** (not the 5-min/2000
  cap's usual early exit - lean-mode attempts never fail outright the way
  completionist ones can, so 2,000 was reached in under a second; running
  the full budget and keeping the *shortest* 2,000 of the 13,246 distinct
  successes found (24,826 attempts, 53.4% success rate) narrowed the
  per-walk length from an initial quick-sample estimate of 104-189 down to
  **103-114 questions per walk (median 111)** - about half the old DFS
  version's ~198-219.
- `Traversal.Bridges` (steps that added no new entity coverage, just needed
  to keep the chain connected - 11-23 per walk) replaces the old entity-level
  `Revisits`, which no longer means anything under this model.
- `liuk leanwalk [out]` runs the full search; `liuk leanwalk-relabel
  <pool.json>` re-derives every step's `via`/`bridgeSource` from the
  *existing* question sequences without re-running the (expensive) search -
  used to ship the citation fix without burning another 5 minutes, safe
  whenever only display logic changed.
- Prototype is live: `go run . serve` + `lean-walk.html`, pulling from
  `lean-walk-pool.json`.

## Open items (not yet decided)

- What happens to the 20 fully-isolated single-entity islands, and to the
  99 + 106 questions with only one or zero tagged entities — none of them
  can support an adjacency walk at all. Not addressed yet.
- How the 4 starting-node options get presented (label by entity name? show
  a preview of the first question? something else?).
- Only one small island (the 17-question holidays cluster) has been spot
  checked the same way as the giant one (4,985 distinct traversals found in
  5,000 attempts — no exhaustion problem there) — worth doing the same check
  on the other 11 before assuming they all behave the same way.
- Everything above was prototyped in Python against a static export of
  `graph-data`; none of it has been ported to Go yet.
