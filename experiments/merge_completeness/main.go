// Experiment: Greedy 2-opt Cycle Merge Completeness
//
// Question: Is greedy 2-opt cycle merge a complete algorithm for finding
// Hamiltonian cycles? Can it always find a Hamiltonian cycle if one exists,
// starting from any valid cycle cover?
//
// Hypothesis: No — it is a local search algorithm that can get stuck at a
// local optimum (a cycle cover with multiple cycles where no single edge-swap
// between any two cycles reduces the cycle count), even when a Hamiltonian
// cycle exists.
//
// This matters because Phase 1 of the planned solver uses cycle-merge as a
// post-processing step. If it's incomplete, the plan needs a fallback.
package main

import (
	"fmt"
	"sort"
	"strings"
)

// --- Graph representation ---

type Graph struct {
	n   int
	adj [][]int
}

func (g *Graph) isEdge(from, to int) bool {
	for _, v := range g.adj[from] {
		if v == to {
			return true
		}
	}
	return false
}

// --- Cycle operations ---

// decomposeCycles decomposes a permutation (assign[i] = recipient of i) into
// directed cycles. Returns them in visitation order.
func decomposeCycles(assign []int) [][]int {
	n := len(assign)
	visited := make([]bool, n)
	var cycles [][]int
	for start := 0; start < n; start++ {
		if visited[start] {
			continue
		}
		var cycle []int
		cur := start
		for !visited[cur] {
			visited[cur] = true
			cycle = append(cycle, cur)
			cur = assign[cur]
		}
		cycles = append(cycles, cycle)
	}
	return cycles
}

// canonicalize returns a canonical string for a cycle cover, suitable for
// deduplication. Cycles are normalized to start at their smallest element,
// then sorted lexicographically.
func canonicalize(assign []int) string {
	cycles := decomposeCycles(assign)
	parts := make([]string, len(cycles))
	for i, c := range cycles {
		minIdx := 0
		for j, v := range c {
			if v < c[minIdx] {
				minIdx = j
			}
		}
		rotated := append(append([]int{}, c[minIdx:]...), c[:minIdx]...)
		strs := make([]string, len(rotated))
		for j, v := range rotated {
			strs[j] = fmt.Sprintf("%d", v)
		}
		parts[i] = strings.Join(strs, "→")
	}
	sort.Strings(parts)
	return strings.Join(parts, " | ")
}

// --- Greedy 2-opt merge ---

// tryMerge2Opt attempts one 2-opt edge swap between cycles c1 and c2.
// It tries both orientations of the swap. Returns the merged assignment
// and true if a valid swap was found.
func tryMerge2Opt(assign []int, g *Graph, c1, c2 []int) ([]int, bool) {
	for _, a := range c1 {
		nextA := assign[a]
		for _, b := range c2 {
			nextB := assign[b]
			// Swap variant 1: replace a→nextA and b→nextB with a→b and nextA→nextB
			if g.isEdge(a, b) && g.isEdge(nextA, nextB) {
				res := make([]int, len(assign))
				copy(res, assign)
				res[a] = b
				res[nextA] = nextB
				return res, true
			}
			// Swap variant 2: replace a→nextA and b→nextB with a→nextB and b→nextA
			if g.isEdge(a, nextB) && g.isEdge(b, nextA) {
				res := make([]int, len(assign))
				copy(res, assign)
				res[a] = nextB
				res[b] = nextA
				return res, true
			}
		}
	}
	return nil, false
}

// greedyMerge iteratively applies 2-opt merges until no more are possible.
// Returns the final assignment and whether it is a Hamiltonian cycle.
func greedyMerge(startAssign []int, g *Graph) ([]int, bool) {
	assign := make([]int, len(startAssign))
	copy(assign, startAssign)

	for {
		cycles := decomposeCycles(assign)
		if len(cycles) == 1 {
			return assign, true
		}
		merged := false
		for i := 0; i < len(cycles) && !merged; i++ {
			for j := i + 1; j < len(cycles) && !merged; j++ {
				if next, ok := tryMerge2Opt(assign, g, cycles[i], cycles[j]); ok {
					assign = next
					merged = true
				}
			}
		}
		if !merged {
			return assign, false // local optimum — stuck
		}
	}
}

// --- Exhaustive enumeration of valid assignments ---

// enumerateValid returns all valid perfect matchings (directed cycle covers)
// for the given graph. Only feasible for small n.
func enumerateValid(g *Graph) [][]int {
	var results [][]int
	assign := make([]int, g.n)
	for i := range assign {
		assign[i] = -1
	}
	usedRecipient := make([]bool, g.n)

	var backtrack func(gifter int)
	backtrack = func(gifter int) {
		if gifter == g.n {
			snapshot := make([]int, g.n)
			copy(snapshot, assign)
			results = append(results, snapshot)
			return
		}
		for _, recipient := range g.adj[gifter] {
			if !usedRecipient[recipient] {
				assign[gifter] = recipient
				usedRecipient[recipient] = true
				backtrack(gifter + 1)
				assign[gifter] = -1
				usedRecipient[recipient] = false
			}
		}
	}
	backtrack(0)
	return results
}

// --- Main experiment ---

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  Experiment: Greedy 2-opt Cycle Merge Completeness          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// -----------------------------------------------------------------------
	// Counterexample graph: n=6
	// Edge list chosen so that:
	//   - A Hamiltonian cycle exists: 0→1→5→4→3→2→0
	//   - Starting from three 2-cycles {0↔1, 2↔3, 4↔5}, no pairwise 2-opt
	//     swap can reduce the cycle count (all merges are blocked by missing
	//     cross-cycle edges).
	// -----------------------------------------------------------------------
	g := &Graph{
		n: 6,
		adj: [][]int{
			{1, 5},    // 0 can give to 1 or 5
			{0, 5},    // 1 can give to 0 or 5
			{3, 0, 4}, // 2 can give to 3, 0, or 4
			{2, 0, 1}, // 3 can give to 2, 0, or 1
			{5, 3},    // 4 can give to 5 or 3
			{4},       // 5 can only give to 4
		},
	}

	fmt.Println("Graph adjacency list:")
	for i, adj := range g.adj {
		fmt.Printf("  node %d → %v\n", i, adj)
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// Part 1: Verify the Hamiltonian cycle exists
	// -----------------------------------------------------------------------
	// Cycle: 0→1→5→4→3→2→0
	// assign[0]=1, assign[1]=5, assign[2]=0, assign[3]=2, assign[4]=3, assign[5]=4
	hamiltonian := []int{1, 5, 0, 2, 3, 4}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Part 1: Known Hamiltonian cycle  0→1→5→4→3→2→0")
	fmt.Printf("  Assignment: %v\n", hamiltonian)

	allEdgesValid := true
	for from, to := range hamiltonian {
		if !g.isEdge(from, to) {
			fmt.Printf("  ✗ INVALID edge %d→%d\n", from, to)
			allEdgesValid = false
		}
	}
	if allEdgesValid {
		fmt.Println("  ✓ All edges are valid")
	}
	hCycles := decomposeCycles(hamiltonian)
	fmt.Printf("  Cycles: %v  (count: %d)\n", hCycles, len(hCycles))
	fmt.Println()

	// -----------------------------------------------------------------------
	// Part 2: Demonstrate the stuck case manually
	// -----------------------------------------------------------------------
	worst := []int{1, 0, 3, 2, 5, 4} // three 2-cycles: 0↔1, 2↔3, 4↔5
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Part 2: Manual trace of greedy merge on worst-case starting point")
	fmt.Printf("  Starting assignment:  %v\n", worst)
	fmt.Printf("  Starting cycles:      %v\n", decomposeCycles(worst))
	fmt.Println()
	fmt.Println("  Checking all pairwise 2-opt swaps:")

	startCycles := decomposeCycles(worst)
	anyMergePossible := false
	for i := 0; i < len(startCycles); i++ {
		for j := i + 1; j < len(startCycles); j++ {
			_, ok := tryMerge2Opt(worst, g, startCycles[i], startCycles[j])
			fmt.Printf("    merge(%v, %v) → possible: %v\n", startCycles[i], startCycles[j], ok)
			if ok {
				anyMergePossible = true
			}
		}
	}
	fmt.Println()
	if !anyMergePossible {
		fmt.Println("  ✗ No 2-opt merge is possible from this starting point.")
		fmt.Println("  ✗ Greedy merge is stuck — yet a Hamiltonian cycle exists.")
	}

	result, isHam := greedyMerge(worst, g)
	fmt.Println()
	fmt.Printf("  greedyMerge result: %v\n", result)
	fmt.Printf("  Is Hamiltonian: %v\n", isHam)
	fmt.Println()

	// -----------------------------------------------------------------------
	// Part 3: Run over ALL valid assignments for this graph
	// -----------------------------------------------------------------------
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Part 3: Exhaustive analysis over all valid cycle covers")

	allAssignments := enumerateValid(g)
	fmt.Printf("  Total valid cycle covers: %d\n", len(allAssignments))

	hamiltonianCount := 0
	stuckCount := 0
	seenCanon := map[string]bool{}
	var stuckExamples [][]int

	for _, a := range allAssignments {
		if len(decomposeCycles(a)) == 1 {
			hamiltonianCount++
			seenCanon[canonicalize(a)] = true
		}
		_, ok := greedyMerge(a, g)
		if !ok {
			stuckCount++
			if len(stuckExamples) < 3 {
				stuckExamples = append(stuckExamples, a)
			}
		}
	}

	fmt.Printf("  Valid assignments that are already Hamiltonian: %d\n", hamiltonianCount)
	fmt.Printf("  Distinct Hamiltonian cycles (canonical): %d\n", len(seenCanon))
	fmt.Printf("  Valid assignments where merge gets STUCK: %d / %d (%.1f%%)\n",
		stuckCount, len(allAssignments),
		float64(stuckCount)/float64(len(allAssignments))*100)
	fmt.Println()

	if len(stuckExamples) > 0 {
		fmt.Println("  Examples of stuck starting assignments:")
		for _, ex := range stuckExamples {
			mergeResult, _ := greedyMerge(ex, g)
			fmt.Printf("    start=%v  cycles=%v  →  stuck_at=%v  cycles=%v\n",
				ex, decomposeCycles(ex), mergeResult, decomposeCycles(mergeResult))
		}
	}

	// -----------------------------------------------------------------------
	// Part 4: 3-opt fix — try simultaneous 3-way merge
	// -----------------------------------------------------------------------
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Part 4: Can a direct Hamiltonian DFS bypass the merge problem?")

	_, foundHam := hamiltonianDFS(g)
	fmt.Printf("  Direct Hamiltonian DFS found a cycle: %v\n", foundHam)
	if foundHam {
		hamAssign, _ := hamiltonianDFS(g)
		fmt.Printf("  Found cycle: %v\n", decomposeCycles(hamAssign))
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// Summary
	// -----------------------------------------------------------------------
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("SUMMARY")
	fmt.Println()
	if stuckCount > 0 {
		fmt.Printf("  Greedy 2-opt merge is NOT complete. It fails on %.1f%% of\n",
			float64(stuckCount)/float64(len(allAssignments))*100)
		fmt.Println("  valid starting assignments for this graph, despite a Hamiltonian")
		fmt.Println("  cycle existing.")
		fmt.Println()
		fmt.Println("  IMPLICATION FOR THE PLAN:")
		fmt.Println("  The solver cannot rely on cycle-merge alone. It must either:")
		fmt.Println("    (a) Use direct Hamiltonian DFS as the primary search method, OR")
		fmt.Println("    (b) Treat LOCAL_OPTIMUM as a trigger for a fresh random restart, OR")
		fmt.Println("    (c) Extend to 3-opt or k-opt merges for completeness.")
	} else {
		fmt.Println("  On this graph, greedy merge succeeded on all valid starting points.")
		fmt.Println("  (This does not prove completeness in general.)")
	}
}

// hamiltonianDFS directly searches for a Hamiltonian cycle via DFS,
// without relying on cycle-merge at all.
func hamiltonianDFS(g *Graph) ([]int, bool) {
	assign := make([]int, g.n)
	for i := range assign {
		assign[i] = -1
	}
	visited := make([]bool, g.n)

	// Always start DFS from node 0 (Hamiltonian cycle is rotation-invariant)
	path := []int{0}
	visited[0] = true

	var dfs func() bool
	dfs = func() bool {
		if len(path) == g.n {
			// Check if the last node can return to the start
			last := path[len(path)-1]
			if g.isEdge(last, path[0]) {
				for i := 0; i < len(path)-1; i++ {
					assign[path[i]] = path[i+1]
				}
				assign[path[len(path)-1]] = path[0]
				return true
			}
			return false
		}
		cur := path[len(path)-1]
		for _, next := range g.adj[cur] {
			if !visited[next] {
				path = append(path, next)
				visited[next] = true
				if dfs() {
					return true
				}
				path = path[:len(path)-1]
				visited[next] = false
			}
		}
		return false
	}

	if dfs() {
		return assign, true
	}
	return nil, false
}
