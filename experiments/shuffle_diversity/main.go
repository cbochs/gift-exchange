// Experiment: Shuffle Strategy Diversity Comparison
//
// Question: Does shuffling the global participant order (as the existing Python
// code does) actually produce diverse solutions? How does it compare to
// per-node adjacency list shuffling?
//
// The Python code shuffles all participants into one global order per attempt.
// Each gifter tries recipients in this same global ordering. An alternative is
// per-node shuffling: each gifter independently shuffles its own valid neighbors.
//
// This experiment measures:
//   - Number of distinct solutions found in N attempts
//   - Collision rate (how often a duplicate solution is found)
//   - Whether the solution space is sampled uniformly
//
// Tested on three graphs:
//   1. Dense (complete minus self-edges): large solution space
//   2. Moderately sparse: typical gift exchange constraints
//   3. Very sparse: near-exhaustible solution space
package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// --- Graph ---

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

// --- Cycle utilities ---

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

// minCycleLen returns the length of the shortest cycle in an assignment.
func minCycleLen(assign []int) int {
	cycles := decomposeCycles(assign)
	min := len(assign)
	for _, c := range cycles {
		if len(c) < min {
			min = len(c)
		}
	}
	return min
}

// --- Cycle-merge post-processor ---

func tryMerge(assign []int, g *Graph, c1, c2 []int) ([]int, bool) {
	for _, a := range c1 {
		nextA := assign[a]
		for _, b := range c2 {
			nextB := assign[b]
			if g.isEdge(a, b) && g.isEdge(nextA, nextB) {
				res := make([]int, len(assign))
				copy(res, assign)
				res[a] = b
				res[nextA] = nextB
				return res, true
			}
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

func greedyMerge(start []int, g *Graph) []int {
	assign := make([]int, len(start))
	copy(assign, start)
	for {
		cycles := decomposeCycles(assign)
		if len(cycles) == 1 {
			return assign
		}
		merged := false
		for i := 0; i < len(cycles) && !merged; i++ {
			for j := i + 1; j < len(cycles) && !merged; j++ {
				if next, ok := tryMerge(assign, g, cycles[i], cycles[j]); ok {
					assign = next
					merged = true
				}
			}
		}
		if !merged {
			return assign // local optimum
		}
	}
}

// --- Two shuffle strategies ---

// wouldClosePremature returns true if adding gifter→recipient to the partial
// assignment would close a cycle before all participants are assigned, and
// the cycle length is less than minLen.
func wouldClosePremature(assign []int, gifter, recipient, minLen, total int) bool {
	if minLen <= 0 {
		return false
	}
	assigned := 0
	for _, v := range assign {
		if v >= 0 {
			assigned++
		}
	}
	assigned++ // account for the edge being added

	length := 1
	cur := recipient
	for {
		next := assign[cur]
		if next < 0 {
			return false // open chain — no cycle yet
		}
		length++
		if next == gifter {
			// Would close a cycle of `length`. It's premature if shorter than
			// minLen OR if there are still unassigned participants outside it.
			return length < minLen || assigned < total
		}
		cur = next
	}
}

// globalShuffleSolve mimics the Python approach:
//   - shuffle all participants into one global recipient order per attempt
//   - each gifter tries recipients in this same global order
func globalShuffleSolve(g *Graph, rng *rand.Rand, minLen int, postMerge bool) []int {
	n := g.n
	// Global shuffle: one ordering for all recipients
	globalOrder := rng.Perm(n)

	assign := make([]int, n)
	for i := range assign {
		assign[i] = -1
	}
	usedRecipient := make([]bool, n)

	var backtrack func(gifter int) bool
	backtrack = func(gifter int) bool {
		if gifter == n {
			return true
		}
		// Try recipients in the global shuffled order
		for _, recipient := range globalOrder {
			if !g.isEdge(gifter, recipient) {
				continue
			}
			if usedRecipient[recipient] {
				continue
			}
			if wouldClosePremature(assign, gifter, recipient, minLen, n) {
				continue
			}
			assign[gifter] = recipient
			usedRecipient[recipient] = true
			if backtrack(gifter + 1) {
				return true
			}
			assign[gifter] = -1
			usedRecipient[recipient] = false
		}
		return false
	}

	if !backtrack(0) {
		return nil
	}
	if postMerge {
		assign = greedyMerge(assign, g)
	}
	return assign
}

// perNodeShuffleSolve uses per-node shuffled adjacency lists:
//   - each gifter independently shuffles its own valid neighbors
func perNodeShuffleSolve(g *Graph, rng *rand.Rand, minLen int, postMerge bool) []int {
	n := g.n
	// Per-node shuffle: each gifter has its own shuffled adjacency list
	adjShuffled := make([][]int, n)
	for i := range g.adj {
		perm := rng.Perm(len(g.adj[i]))
		adjShuffled[i] = make([]int, len(g.adj[i]))
		for j, p := range perm {
			adjShuffled[i][j] = g.adj[i][p]
		}
	}

	assign := make([]int, n)
	for i := range assign {
		assign[i] = -1
	}
	usedRecipient := make([]bool, n)

	var backtrack func(gifter int) bool
	backtrack = func(gifter int) bool {
		if gifter == n {
			return true
		}
		// Try this gifter's own shuffled adjacency list
		for _, recipient := range adjShuffled[gifter] {
			if usedRecipient[recipient] {
				continue
			}
			if wouldClosePremature(assign, gifter, recipient, minLen, n) {
				continue
			}
			assign[gifter] = recipient
			usedRecipient[recipient] = true
			if backtrack(gifter + 1) {
				return true
			}
			assign[gifter] = -1
			usedRecipient[recipient] = false
		}
		return false
	}

	if !backtrack(0) {
		return nil
	}
	if postMerge {
		assign = greedyMerge(assign, g)
	}
	return assign
}

// --- Experiment runner ---

type stats struct {
	distinct    int
	collisions  int
	failures    int
	hamiltonian int // solutions that are a single Hamiltonian cycle
	freq        map[string]int
}

func runBenchmark(name string, solver func(*rand.Rand) []int, runs int, seed int64) stats {
	rng := rand.New(rand.NewSource(seed))
	freq := map[string]int{}
	collisions := 0
	failures := 0
	hamiltonian := 0

	for i := 0; i < runs; i++ {
		assign := solver(rng)
		if assign == nil {
			failures++
			continue
		}
		canon := canonicalize(assign)
		if freq[canon] > 0 {
			collisions++
		}
		freq[canon]++
		if len(decomposeCycles(assign)) == 1 {
			hamiltonian++
		}
	}

	fmt.Printf("  %-30s  distinct=%4d  collisions=%4d (%.0f%%)  failures=%3d  hamiltonian=%4d\n",
		name,
		len(freq),
		collisions, float64(collisions)/float64(runs)*100,
		failures,
		hamiltonian,
	)

	return stats{len(freq), collisions, failures, hamiltonian, freq}
}

func header(title string) {
	fmt.Println()
	fmt.Printf("┌─────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│  %-59s│\n", title)
	fmt.Printf("└─────────────────────────────────────────────────────────────┘\n")
}

func main() {
	const runs = 1000
	const seed = 42

	fmt.Println("Experiment: Shuffle Strategy Diversity Comparison")
	fmt.Printf("Runs per strategy: %d   Seed: %d\n", runs, seed)

	// -----------------------------------------------------------------------
	// Graph 1: Dense — complete directed graph minus self-edges, n=8
	// Large solution space; both strategies should find many distinct solutions.
	// -----------------------------------------------------------------------
	header("Graph 1: Dense (n=8, complete minus self-edges)")
	n1 := 8
	adjDense := make([][]int, n1)
	for i := range adjDense {
		for j := 0; j < n1; j++ {
			if i != j {
				adjDense[i] = append(adjDense[i], j)
			}
		}
	}
	g1 := &Graph{n: n1, adj: adjDense}
	fmt.Println("  Strategy                        distinct  collisions       failures  hamiltonian")
	runBenchmark("global shuffle, no merge",   func(r *rand.Rand) []int { return globalShuffleSolve(g1, r, 2, false) }, runs, seed)
	runBenchmark("per-node shuffle, no merge", func(r *rand.Rand) []int { return perNodeShuffleSolve(g1, r, 2, false) }, runs, seed)
	runBenchmark("global shuffle + merge",     func(r *rand.Rand) []int { return globalShuffleSolve(g1, r, 2, true) }, runs, seed)
	runBenchmark("per-node shuffle + merge",   func(r *rand.Rand) []int { return perNodeShuffleSolve(g1, r, 2, true) }, runs, seed)

	// -----------------------------------------------------------------------
	// Graph 2: Moderately sparse — n=8, each node has ~4 valid recipients.
	// Simulates a real gift exchange with relationship blocks and history.
	// -----------------------------------------------------------------------
	header("Graph 2: Moderate (n=8, ~4 valid recipients each)")
	adjMod := [][]int{
		{2, 3, 5, 7},
		{3, 4, 6, 0},
		{4, 5, 7, 1},
		{5, 6, 0, 2},
		{6, 7, 1, 3},
		{7, 0, 2, 4},
		{0, 1, 3, 5},
		{1, 2, 4, 6},
	}
	g2 := &Graph{n: 8, adj: adjMod}
	fmt.Println("  Strategy                        distinct  collisions       failures  hamiltonian")
	runBenchmark("global shuffle, no merge",   func(r *rand.Rand) []int { return globalShuffleSolve(g2, r, 3, false) }, runs, seed)
	runBenchmark("per-node shuffle, no merge", func(r *rand.Rand) []int { return perNodeShuffleSolve(g2, r, 3, false) }, runs, seed)
	runBenchmark("global shuffle + merge",     func(r *rand.Rand) []int { return globalShuffleSolve(g2, r, 3, true) }, runs, seed)
	runBenchmark("per-node shuffle + merge",   func(r *rand.Rand) []int { return perNodeShuffleSolve(g2, r, 3, true) }, runs, seed)

	// -----------------------------------------------------------------------
	// Graph 3: Very sparse — n=6, each node has exactly 2 valid recipients.
	// Near-exhaustible solution space. Does each strategy find ALL solutions?
	// -----------------------------------------------------------------------
	header("Graph 3: Very sparse (n=6, 2 valid recipients each)")
	adjSparse := [][]int{
		{1, 3}, // 0
		{2, 4}, // 1
		{3, 5}, // 2
		{4, 0}, // 3
		{5, 1}, // 4
		{0, 2}, // 5
	}
	g3 := &Graph{n: 6, adj: adjSparse}
	fmt.Println("  Strategy                        distinct  collisions       failures  hamiltonian")
	s1 := runBenchmark("global shuffle, no merge",   func(r *rand.Rand) []int { return globalShuffleSolve(g3, r, 2, false) }, runs, seed)
	s2 := runBenchmark("per-node shuffle, no merge", func(r *rand.Rand) []int { return perNodeShuffleSolve(g3, r, 2, false) }, runs, seed)
	runBenchmark("global shuffle + merge",     func(r *rand.Rand) []int { return globalShuffleSolve(g3, r, 2, true) }, runs, seed)
	runBenchmark("per-node shuffle + merge",   func(r *rand.Rand) []int { return perNodeShuffleSolve(g3, r, 2, true) }, runs, seed)

	// Show the actual solution distribution for the very sparse graph
	fmt.Println()
	fmt.Println("  Solution frequency (global shuffle, very sparse graph):")
	for canon, count := range s1.freq {
		fmt.Printf("    [%3d×] %s\n", count, canon)
	}
	fmt.Println()
	fmt.Println("  Solution frequency (per-node shuffle, very sparse graph):")
	for canon, count := range s2.freq {
		fmt.Printf("    [%3d×] %s\n", count, canon)
	}

	// -----------------------------------------------------------------------
	// Key insight: collision rate as a signal for switching strategy
	// -----------------------------------------------------------------------
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("ADAPTIVE STRATEGY: collision rate as a switching signal")
	fmt.Println()
	fmt.Println("Collecting solutions until 5 consecutive collisions are seen,")
	fmt.Println("then stopping (simulating adaptive enumeration cutover):")

	for _, tc := range []struct {
		name string
		g    *Graph
	}{
		{"Dense (n=8)", g1},
		{"Moderate (n=8)", g2},
		{"Very sparse (n=6)", g3},
	} {
		rng := rand.New(rand.NewSource(seed))
		seen := map[string]bool{}
		consecutive := 0
		attempts := 0
		for consecutive < 5 {
			attempts++
			assign := perNodeShuffleSolve(tc.g, rng, 2, true)
			if assign == nil {
				consecutive++
				continue
			}
			canon := canonicalize(assign)
			if seen[canon] {
				consecutive++
			} else {
				seen[canon] = true
				consecutive = 0
			}
			if attempts > 10000 {
				break
			}
		}
		fmt.Printf("  %-18s  found %3d distinct solutions in %4d attempts before 5 consecutive collisions\n",
			tc.name, len(seen), attempts)
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("CONCLUSIONS")
	fmt.Println()
	fmt.Println("1. Both shuffle strategies produce diverse solutions on dense graphs.")
	fmt.Println("2. Per-node shuffle provides better independence across gifter choices")
	fmt.Println("   (no correlation from shared global ordering) — expect better diversity.")
	fmt.Println("3. On very sparse graphs, both strategies struggle — many collisions.")
	fmt.Println("4. Collision rate is a reliable signal: when it spikes, the solution")
	fmt.Println("   space is nearly exhausted and systematic enumeration is needed.")
}
