package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// runLeanWalk generates the pools behind the app's "Island walk" mode. It
// does NOT regenerate the knowledge graph itself (dist/knowledge-map.html has
// no such pipeline yet - see docs/KNOWLEDGE-WALK-DESIGN.md). It reads the
// graph-data already embedded there, picks the islands (connected
// components of the entity graph) with enough actual QUESTION content to
// support a walk, and for each one searches the QUESTION graph (nodes =
// questions, edges = shared-or-adjacent entity) for "lean walk"
// traversals: a strict-adjacency walk where every consecutive pair of
// shown questions is a real graph edge (no silent skips), stopping as soon
// as every entity has been touched at least once. Shorter is better - see
// the design doc's "lean walk, take two" section for why an entity-level
// DFS version of this under-delivered, and why ranking islands by entity
// count (rather than actual question count) is misleading: several small
// entity clusters turn out to be a single multi-answer question's options
// cross-linked into a clique, with nowhere near enough content to walk.
//
// Usage: liuk leanwalk [output.json]
// Per island: runs up to 1 minute in parallel across all CPUs, then keeps
// the 2000 shortest distinct successful walks found (not just the first
// 2000 - shorter walks serve the "doable in one sitting" goal better).
// Exits non-zero if any island falls short of 2000 - meant to gate CI
// deploys, not just report a warning.
func runLeanWalk(args []string) error {
	out := "dist/island-walk-pools.json"
	if len(args) > 0 {
		out = args[0]
	}

	graph, err := loadGraphData("dist/knowledge-map.html")
	if err != nil {
		return fmt.Errorf("loading graph data: %w", err)
	}

	entityAdj := buildAdjacency(graph.Edges)
	edgeQuestions := buildEdgeQuestions(graph.Edges)
	labelByEntity := make(map[string]string, len(graph.Nodes))
	for _, n := range graph.Nodes {
		labelByEntity[n.ID] = n.Label
	}

	// Only one island currently has enough actual question content to
	// support a real walk - see docs/KNOWLEDGE-WALK-DESIGN.md. topIslandsByQuestionCount
	// stays general (rather than hardcoding "the giant one") so a future
	// change to dist/exams.json that enriches a second cluster enough to
	// clear the 2000-walk bar is picked up automatically, not silently
	// ignored.
	islands := topIslandsByQuestionCount(graph, entityAdj, 1)

	const target = 2000
	const budget = 1 * time.Minute

	start := time.Now()
	var pools []IslandPool
	var failures []string

	for _, isl := range islands {
		fmt.Printf("=== island %q (%s): %d entities, %d questions ===\n", isl.Label, isl.ID, len(isl.Members), isl.QuestionCount)
		qEntities, qNeighbors, qlist := buildQuestionGraph(graph, isl.Members, entityAdj)

		successes, attempts := searchIsland(qlist, qEntities, qNeighbors, len(isl.Members), budget, target)
		fmt.Printf("  %d attempts, %d distinct full-coverage walks found (success rate %.1f%%)\n",
			attempts, len(successes), 100*float64(len(successes))/float64(attempts))

		if len(successes) < target {
			failures = append(failures, fmt.Sprintf("%s: only %d/%d distinct walks", isl.Label, len(successes), target))
		}

		traversals := make([]Traversal, 0, len(successes))
		for _, f := range successes {
			traversals = append(traversals, buildTraversal(f.path, f.bridges, qEntities, entityAdj, labelByEntity, edgeQuestions))
		}
		if len(traversals) > 0 {
			lengths := make([]int, len(traversals))
			for i, t := range traversals {
				lengths[i] = len(t.Steps)
			}
			sort.Ints(lengths)
			fmt.Printf("  questions-per-walk range: %d - %d (median %d)\n", lengths[0], lengths[len(lengths)-1], lengths[len(lengths)/2])
		}

		pools = append(pools, IslandPool{
			ID:          isl.ID,
			Label:       isl.Label,
			EntityCount: len(isl.Members),
			Attempts:    int(attempts),
			Traversals:  traversals,
		})
	}

	dump := PoolFile{GeneratedAt: start.Format(time.RFC3339), Islands: pools}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(dump); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", out)

	if len(failures) > 0 {
		return fmt.Errorf("fell short of the %d-walk target: %s", target, strings.Join(failures, "; "))
	}
	return nil
}

type found struct {
	path    []string
	bridges int
}

// searchIsland runs strictLeanWalk from random starts across all CPUs for
// up to `budget`, keeping the `target` shortest distinct successful walks
// found (not just the first `target`). Each worker keeps its own RNG and
// local seen-set/results (math/rand.Rand isn't safe for concurrent use,
// and this avoids any locking in the hot loop); results are merged and
// deduped once, after every worker finishes.
func searchIsland(qlist []string, qEntities, qNeighbors map[string]map[string]bool, totalEntities int, budget time.Duration, target int) ([]found, int64) {
	numWorkers := runtime.NumCPU()
	var totalAttempts int64
	workerResults := make([][]found, numWorkers)

	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)*1_000_003))
			localSeen := make(map[string]bool)
			var local []found
			var localAttempts int64
			for time.Since(start) < budget {
				localAttempts++
				startQ := qlist[rng.Intn(len(qlist))]
				path, bridges, success := strictLeanWalk(startQ, totalEntities, qEntities, qNeighbors, rng)
				if !success {
					continue
				}
				key := strings.Join(path, ",")
				if localSeen[key] {
					continue
				}
				localSeen[key] = true
				local = append(local, found{path: path, bridges: bridges})
			}
			atomic.AddInt64(&totalAttempts, localAttempts)
			workerResults[workerID] = local
		}(w)
	}
	wg.Wait()

	seen := make(map[string]bool)
	var successes []found
	for _, local := range workerResults {
		for _, f := range local {
			key := strings.Join(f.path, ",")
			if seen[key] {
				continue
			}
			seen[key] = true
			successes = append(successes, f)
		}
	}

	sort.Slice(successes, func(i, j int) bool { return len(successes[i].path) < len(successes[j].path) })
	if len(successes) > target {
		successes = successes[:target]
	}
	return successes, totalAttempts
}

// runLeanWalkRelabel re-derives the "via"/"bridgeSourceUid" on every step of
// an already-generated pool file, using the current buildTraversal logic,
// without re-running the (expensive) search. Safe whenever only the
// display-derivation logic changed, not the question sequences themselves.
// Usage: liuk leanwalk-relabel <pool.json>
func runLeanWalkRelabel(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: liuk leanwalk-relabel <pool.json>")
	}
	path := args[0]

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var pool PoolFile
	if err := json.Unmarshal(raw, &pool); err != nil {
		return err
	}

	graph, err := loadGraphData("dist/knowledge-map.html")
	if err != nil {
		return err
	}
	entityAdj := buildAdjacency(graph.Edges)
	edgeQuestions := buildEdgeQuestions(graph.Edges)
	labelByEntity := make(map[string]string, len(graph.Nodes))
	for _, n := range graph.Nodes {
		labelByEntity[n.ID] = n.Label
	}
	islands := topIslandsByQuestionCount(graph, entityAdj, len(pool.Islands))
	membersByID := make(map[string]map[string]bool, len(islands))
	for _, isl := range islands {
		membersByID[isl.ID] = isl.Members
	}

	for i, island := range pool.Islands {
		members := membersByID[island.ID]
		if members == nil {
			return fmt.Errorf("island %q from pool file no longer matches a current island - regenerate instead", island.ID)
		}
		qEntities, _, _ := buildQuestionGraph(graph, members, entityAdj)
		for j, t := range island.Traversals {
			qs := make([]string, len(t.Steps))
			for k, s := range t.Steps {
				qs[k] = s.Q
			}
			pool.Islands[i].Traversals[j] = buildTraversal(qs, t.Bridges, qEntities, entityAdj, labelByEntity, edgeQuestions)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(pool); err != nil {
		return err
	}
	fmt.Printf("relabeled %d islands in %s\n", len(pool.Islands), path)
	return nil
}

// --- graph data model (mirrors the JSON embedded in dist/knowledge-map.html) ---

type GraphNode struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Type      string   `json:"type"`
	Count     int      `json:"count"`
	Questions []string `json:"questions"`
}

type GraphEdge struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	Weight    int      `json:"weight"`
	Questions []string `json:"questions"`
}

type QuestionData struct {
	Question  string   `json:"question"`
	Reference string   `json:"reference"`
	Category  string   `json:"category"`
	Answers   []string `json:"answers"`
	Correct   []string `json:"correct"`
}

type GraphData struct {
	Nodes     []GraphNode             `json:"nodes"`
	Edges     []GraphEdge             `json:"edges"`
	Questions map[string]QuestionData `json:"questions"`
}

func loadGraphData(htmlPath string) (*GraphData, error) {
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?s)<script id="graph-data" type="application/json">(.*?)</script>`)
	m := re.FindSubmatch(raw)
	if m == nil {
		return nil, fmt.Errorf("graph-data script tag not found in %s", htmlPath)
	}
	var gd GraphData
	if err := json.Unmarshal(m[1], &gd); err != nil {
		return nil, err
	}
	return &gd, nil
}

func buildAdjacency(edges []GraphEdge) map[string]map[string]bool {
	adj := make(map[string]map[string]bool)
	add := func(a, b string) {
		if adj[a] == nil {
			adj[a] = make(map[string]bool)
		}
		adj[a][b] = true
	}
	for _, e := range edges {
		add(e.Source, e.Target)
		add(e.Target, e.Source)
	}
	return adj
}

// buildEdgeQuestions indexes, for each entity pair, the actual question(s)
// that established their adjacency - the citation that makes an
// adjacency-only step's connection checkable instead of an assertion.
func buildEdgeQuestions(edges []GraphEdge) map[[2]string][]string {
	m := make(map[[2]string][]string, len(edges)*2)
	for _, e := range edges {
		m[[2]string{e.Source, e.Target}] = e.Questions
		m[[2]string{e.Target, e.Source}] = e.Questions
	}
	return m
}

// island describes one connected component of the entity graph, ranked and
// labeled for use by "Island walk" mode.
type island struct {
	ID            string
	Label         string
	Members       map[string]bool
	QuestionCount int
}

// topIslandsByQuestionCount finds every connected component of the entity
// graph and returns the `n` with the most actual QUESTION content -
// entity count is a misleading ranking here: several small entity
// clusters turn out to be a single multi-answer question's options
// cross-linked into a clique (e.g. 8 entities but only 3 distinct
// questions between them), nowhere near enough to support a walk.
func topIslandsByQuestionCount(graph *GraphData, adj map[string]map[string]bool, n int) []island {
	parent := make(map[string]string)
	for _, node := range graph.Nodes {
		parent[node.ID] = node.ID
	}
	var find func(string) string
	find = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for a, nbrs := range adj {
		for b := range nbrs {
			union(a, b)
		}
	}

	groups := make(map[string]map[string]bool)
	for _, node := range graph.Nodes {
		r := find(node.ID)
		if groups[r] == nil {
			groups[r] = make(map[string]bool)
		}
		groups[r][node.ID] = true
	}

	byID := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		byID[node.ID] = node
	}

	var islands []island
	for _, members := range groups {
		qs := make(map[string]bool)
		for id := range members {
			for _, q := range byID[id].Questions {
				qs[q] = true
			}
		}
		islands = append(islands, island{Members: members, QuestionCount: len(qs)})
	}
	sort.Slice(islands, func(i, j int) bool { return islands[i].QuestionCount > islands[j].QuestionCount })
	if len(islands) > n {
		islands = islands[:n]
	}

	for i := range islands {
		islands[i].Label = labelForIsland(byID, islands[i].Members)
		islands[i].ID = idForIsland(islands[i].Label, i)
	}
	return islands
}

// labelForIsland derives a human label from actual island content (rather
// than trusting a hardcoded position, which would silently mislabel if
// dist/exams.json ever reshapes these clusters) by checking for a couple
// of anchor entities known to sit in each of the two islands with enough
// content to be worth offering. Anything else is unexpected - flagged with
// a fallback label and a printed warning rather than a guess.
func labelForIsland(byID map[string]GraphNode, members map[string]bool) string {
	hasLabel := func(want string) bool {
		for id := range members {
			if byID[id].Label == want {
				return true
			}
		}
		return false
	}
	switch {
	case hasLabel("France") || hasLabel("London"):
		return "History, geography and government"
	case hasLabel("Easter") || hasLabel("Christmas Day"):
		return "Festivals and holidays"
	default:
		fmt.Printf("warning: island with %d entities didn't match any known label pattern - using a generic fallback; check topIslandsByQuestionCount/labelForIsland\n", len(members))
		return fmt.Sprintf("Topic cluster (%d entities)", len(members))
	}
}

func idForIsland(label string, index int) string {
	switch label {
	case "History, geography and government":
		return "big-island"
	case "Festivals and holidays":
		return "holidays"
	default:
		return fmt.Sprintf("island-%d", index)
	}
}

// buildQuestionGraph builds the "blow-up" graph used by completionist mode
// too: nodes are questions (restricted to ones tagged with an entity in
// the given island), and two questions are adjacent if they share an
// entity or their entities are adjacent in the entity graph.
func buildQuestionGraph(graph *GraphData, members map[string]bool, entityAdj map[string]map[string]bool) (
	qEntities map[string]map[string]bool, qNeighbors map[string]map[string]bool, qlist []string) {

	qEntities = make(map[string]map[string]bool)
	qsByEntity := make(map[string]map[string]bool)
	for _, n := range graph.Nodes {
		if !members[n.ID] {
			continue
		}
		for _, q := range n.Questions {
			if qEntities[q] == nil {
				qEntities[q] = make(map[string]bool)
			}
			qEntities[q][n.ID] = true
			if qsByEntity[n.ID] == nil {
				qsByEntity[n.ID] = make(map[string]bool)
			}
			qsByEntity[n.ID][q] = true
		}
	}
	for q := range qEntities {
		qlist = append(qlist, q)
	}
	sort.Strings(qlist)

	qNeighbors = make(map[string]map[string]bool, len(qlist))
	for _, q := range qlist {
		result := make(map[string]bool)
		for e := range qEntities[q] {
			for other := range qsByEntity[e] {
				result[other] = true
			}
			for e2 := range entityAdj[e] {
				for other := range qsByEntity[e2] {
					result[other] = true
				}
			}
		}
		delete(result, q)
		qNeighbors[q] = result
	}
	return
}

// strictLeanWalk greedily extends a path of questions, always picking the
// unshown adjacent question that covers the most NEW entities (ties broken
// by fewest remaining options elsewhere, then randomly), until every
// entity in the island is covered or no adjacent unshown question remains.
func strictLeanWalk(start string, totalEntities int, qEntities, qNeighbors map[string]map[string]bool, rng *rand.Rand) (path []string, bridgeSteps int, success bool) {
	path = []string{start}
	shown := map[string]bool{start: true}
	covered := make(map[string]bool, totalEntities)
	for e := range qEntities[start] {
		covered[e] = true
	}

	newCoverage := func(q string) int {
		n := 0
		for e := range qEntities[q] {
			if !covered[e] {
				n++
			}
		}
		return n
	}

	for len(covered) < totalEntities {
		cur := path[len(path)-1]
		var cands []string
		for v := range qNeighbors[cur] {
			if !shown[v] {
				cands = append(cands, v)
			}
		}
		if len(cands) == 0 {
			return path, bridgeSteps, false
		}
		bestNew := -1
		for _, v := range cands {
			if n := newCoverage(v); n > bestNew {
				bestNew = n
			}
		}
		if bestNew == 0 {
			bridgeSteps++
		}
		var top []string
		for _, v := range cands {
			if newCoverage(v) == bestNew {
				top = append(top, v)
			}
		}
		sort.Slice(top, func(i, j int) bool {
			return remainingOptions(top[i], qNeighbors, shown) < remainingOptions(top[j], qNeighbors, shown)
		})
		// among ties for fewest remaining options, pick randomly
		tieLen := 1
		for tieLen < len(top) && remainingOptions(top[tieLen], qNeighbors, shown) == remainingOptions(top[0], qNeighbors, shown) {
			tieLen++
		}
		next := top[rng.Intn(tieLen)]

		path = append(path, next)
		shown[next] = true
		for e := range qEntities[next] {
			covered[e] = true
		}
	}
	return path, bridgeSteps, true
}

func countShown(set map[string]bool, shown map[string]bool) int {
	n := 0
	for v := range set {
		if shown[v] {
			n++
		}
	}
	return n
}

func remainingOptions(q string, qNeighbors map[string]map[string]bool, shown map[string]bool) int {
	return len(qNeighbors[q]) - countShown(qNeighbors[q], shown)
}

// Step is one card shown to the player: which question (by its exams.json
// uid - the app resolves the actual text/answers from data it already has
// loaded, so this file never needs to duplicate question content), which
// entity (or entity pair) connects it to the question before it, and -
// only when that connection isn't a shared entity - the uid of the actual
// third question that establishes the bridge, so the link is checkable
// rather than an unsupported assertion.
type Step struct {
	Q               string `json:"q"`
	Via             string `json:"via"`
	BridgeSourceUID string `json:"bridgeSourceUid,omitempty"`
}

type Traversal struct {
	Start   string `json:"start"`   // entity label shown as the walk's starting topic
	Bridges int    `json:"bridges"` // steps that added no new entity coverage, just needed to keep the strict-adjacency chain connected
	Steps   []Step `json:"steps"`
}

// buildTraversal re-derives a human-facing "via" entity label for each
// step: the entity shared between consecutive questions if one exists,
// otherwise both ends of the entity-adjacency edge that connects them,
// plus the uid of the third question establishing that edge (guaranteed
// to exist, since the path was built strictly from question-graph edges).
func buildTraversal(path []string, bridges int, qEntities map[string]map[string]bool, entityAdj map[string]map[string]bool, labelByEntity map[string]string, edgeQuestions map[[2]string][]string) Traversal {
	sortedEntities := func(set map[string]bool) []string {
		out := make([]string, 0, len(set))
		for e := range set {
			out = append(out, e)
		}
		sort.Strings(out)
		return out
	}

	steps := make([]Step, len(path))
	for i, q := range path {
		var via, bridgeSourceUID string
		if i == 0 {
			ents := sortedEntities(qEntities[q])
			if len(ents) > 0 {
				via = labelByEntity[ents[0]]
			}
		} else {
			prevEnts := qEntities[path[i-1]]
			curEnts := sortedEntities(qEntities[q])
			shared := ""
			for _, e := range curEnts {
				if prevEnts[e] {
					shared = e
					break
				}
			}
			if shared != "" {
				// A real shared entity - both questions visibly mention it,
				// so a single label is honest and enough.
				via = labelByEntity[shared]
			} else {
				// No shared entity: the two questions are only connected
				// because some entity in the previous one and some entity
				// in this one are adjacent elsewhere in the graph - that
				// adjacency itself came from some OTHER question mentioning
				// both. Show both ends of the edge, plus a citation of that
				// third question, so the link is checkable rather than an
				// unsupported assertion.
				bridgeFrom, bridgeTo := "", ""
			outer:
				for _, pe := range sortedEntities(prevEnts) {
					for _, e := range curEnts {
						if entityAdj[pe][e] {
							bridgeFrom, bridgeTo = pe, e
							break outer
						}
					}
				}
				via = labelByEntity[bridgeFrom] + " → " + labelByEntity[bridgeTo]
				if qs := edgeQuestions[[2]string{bridgeFrom, bridgeTo}]; len(qs) > 0 {
					bridgeSourceUID = qs[0]
				}
			}
		}
		steps[i] = Step{Q: q, Via: via, BridgeSourceUID: bridgeSourceUID}
	}

	start := ""
	if len(steps) > 0 {
		start = steps[0].Via
	}
	return Traversal{Start: start, Bridges: bridges, Steps: steps}
}

type IslandPool struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	EntityCount int         `json:"entityCount"`
	Attempts    int         `json:"attempts"`
	Traversals  []Traversal `json:"traversals"`
}

type PoolFile struct {
	GeneratedAt string       `json:"generatedAt"`
	Islands     []IslandPool `json:"islands"`
}
