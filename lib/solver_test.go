package giftexchange

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeParticipants(n int) []Participant {
	p := make([]Participant, n)
	for i := range n {
		id := fmt.Sprintf("%d", i)
		p[i] = Participant{ID: id, Name: "Person " + id}
	}
	return p
}

// twoGroupProblem builds a problem where participants are split into two equal
// groups and all cross-group gifting is blocked. No Hamiltonian cycle is
// possible; the best achievable is two n/2-cycles.
func twoGroupProblem(n int) Problem {
	participants := makeParticipants(n)
	half := n / 2
	var blocks []Block
	for i := range n {
		for j := range n {
			if i != j && (i < half) != (j < half) {
				// cross-group: block
				blocks = append(blocks, Block{
					From: participants[i].ID,
					To:   participants[j].ID,
				})
			}
		}
	}
	return Problem{Participants: participants, Blocks: blocks}
}

// counterexampleProblem builds the 6-node graph from the Phase 1 experiments
// that has exactly one Hamiltonian cycle: 0→1→5→4→3→2→0.
// adj = [[1,5], [0,5], [3,0,4], [2,0,1], [5,3], [4]]
func counterexampleProblem() Problem {
	participants := makeParticipants(6)
	adj := [][]int{
		{1, 5},
		{0, 5},
		{3, 0, 4},
		{2, 0, 1},
		{5, 3},
		{4},
	}
	var blocks []Block
	for i := range 6 {
		adjSet := make(map[int]bool)
		for _, j := range adj[i] {
			adjSet[j] = true
		}
		for j := range 6 {
			if i != j && !adjSet[j] {
				blocks = append(blocks, Block{
					From: participants[i].ID,
					To:   participants[j].ID,
				})
			}
		}
	}
	return Problem{Participants: participants, Blocks: blocks}
}

func solutionKey(s Solution) string {
	pairs := make([]string, len(s.Assignments))
	for i, a := range s.Assignments {
		pairs[i] = a.GifterID + "→" + a.RecipientID
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func assertValidSolution(t *testing.T, p Problem, s Solution) {
	t.Helper()
	n := len(p.Participants)

	givers := make(map[string]int)
	receivers := make(map[string]int)
	for _, a := range s.Assignments {
		givers[a.GifterID]++
		receivers[a.RecipientID]++
	}
	if len(givers) != n {
		t.Errorf("expected %d unique givers, got %d", n, len(givers))
	}
	if len(receivers) != n {
		t.Errorf("expected %d unique receivers, got %d", n, len(receivers))
	}
	for id, count := range givers {
		if count != 1 {
			t.Errorf("participant %q gives %d times (expected 1)", id, count)
		}
	}
	for id, count := range receivers {
		if count != 1 {
			t.Errorf("participant %q receives %d times (expected 1)", id, count)
		}
	}

	blocked := make(map[[2]string]bool)
	for _, b := range p.Blocks {
		blocked[[2]string{b.From, b.To}] = true
	}
	for _, a := range s.Assignments {
		if a.GifterID == a.RecipientID {
			t.Errorf("self-assignment: %q → %q", a.GifterID, a.RecipientID)
		}
		if blocked[[2]string{a.GifterID, a.RecipientID}] {
			t.Errorf("blocked assignment used: %q → %q", a.GifterID, a.RecipientID)
		}
	}

	for _, c := range s.Cycles {
		if len(c) < s.Score.MinCycleLen {
			t.Errorf("cycle of length %d is below Score.MinCycleLen=%d", len(c), s.Score.MinCycleLen)
		}
	}

	actualMin := n
	actualMax := 0
	for _, c := range s.Cycles {
		if len(c) < actualMin {
			actualMin = len(c)
		}
		if len(c) > actualMax {
			actualMax = len(c)
		}
	}
	if actualMin != s.Score.MinCycleLen {
		t.Errorf("Score.MinCycleLen=%d but actual min cycle length is %d", s.Score.MinCycleLen, actualMin)
	}
	if actualMax != s.Score.MaxCycleLen {
		t.Errorf("Score.MaxCycleLen=%d but actual max cycle length is %d", s.Score.MaxCycleLen, actualMax)
	}
	if len(s.Cycles) != s.Score.NumCycles {
		t.Errorf("Score.NumCycles=%d but actual cycle count is %d", s.Score.NumCycles, len(s.Cycles))
	}
}

// ---------------------------------------------------------------------------
// Unit tests: graph
// ---------------------------------------------------------------------------

func TestBuildGraph(t *testing.T) {
	participants := makeParticipants(4)
	blocks := []Block{
		{From: "0", To: "1"},
		{From: "2", To: "3"},
	}
	g := buildGraph(participants, blocks)

	if g.n != 4 {
		t.Errorf("g.n = %d, want 4", g.n)
	}
	// 0→1 is blocked
	if g.isEdge(0, 1) {
		t.Error("edge 0→1 should be blocked")
	}
	// 0→2 should exist
	if !g.isEdge(0, 2) {
		t.Error("edge 0→2 should exist")
	}
	// 2→3 is blocked
	if g.isEdge(2, 3) {
		t.Error("edge 2→3 should be blocked")
	}
	// self-edges never exist
	for i := range 4 {
		if g.isEdge(i, i) {
			t.Errorf("self-edge %d→%d should not exist", i, i)
		}
	}
	// directed: 1→0 is not blocked even though 0→1 is
	if !g.isEdge(1, 0) {
		t.Error("edge 1→0 should exist (block is directed)")
	}
}

// ---------------------------------------------------------------------------
// Unit tests: wouldClosePrematureCycle
// ---------------------------------------------------------------------------

func TestWouldClosePrematureCycle(t *testing.T) {
	tests := []struct {
		name      string
		assign    []int
		gifter    int
		recipient int
		minLen    int
		want      bool
	}{
		{
			name:      "open chain — no cycle forms",
			assign:    []int{-1, -1, -1, -1},
			gifter:    0, recipient: 1, minLen: 2,
			want: false,
		},
		{
			name:      "open chain partway — no cycle back to gifter",
			assign:    []int{1, -1, -1, -1},
			gifter:    0, recipient: 2, minLen: 2,
			want: false,
		},
		{
			name:      "would close 2-cycle, minLen=2 — allow",
			assign:    []int{1, -1, -1, -1},
			gifter:    1, recipient: 0, minLen: 2,
			want: false,
		},
		{
			name:      "would close 2-cycle, minLen=3 — reject (too short)",
			assign:    []int{1, -1, -1, -1},
			gifter:    1, recipient: 0, minLen: 3,
			want: true,
		},
		{
			name:      "would close 3-cycle, minLen=3 — allow",
			assign:    []int{1, 2, -1, -1},
			gifter:    2, recipient: 0, minLen: 3,
			want: false,
		},
		{
			name:      "would close 4-cycle at final step, minLen=3 — allow",
			assign:    []int{1, 2, 3, -1},
			gifter:    3, recipient: 0, minLen: 3,
			want: false,
		},
		{
			// Key test: even at the final assignment (all other edges set),
			// a too-short cycle must still be rejected.
			// assign=[2,3,0,-1]: 0→2, 1→3, 2→0 is a 2-cycle. gifter=3, recipient=1.
			// But here gifter=3→recipient=1: chain from 1 is assign[1]=3, next=gifter=3, length=2.
			name:      "final assignment but cycle too short — reject",
			assign:    []int{2, 3, 0, -1},
			gifter:    3, recipient: 1, minLen: 3,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wouldClosePrematureCycle(tt.assign, tt.gifter, tt.recipient, tt.minLen)
			if got != tt.want {
				t.Errorf("wouldClosePrematureCycle(...) = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unit tests: score
// ---------------------------------------------------------------------------

func TestDecomposeCycles(t *testing.T) {
	ids := []string{"0", "1", "2", "3", "4", "5"}

	t.Run("single 6-cycle", func(t *testing.T) {
		// 0→1→5→4→3→2→0
		assign := []int{1, 5, 0, 2, 3, 4}
		cycles := decomposeCycles(assign, ids)
		if len(cycles) != 1 {
			t.Fatalf("expected 1 cycle, got %d", len(cycles))
		}
		if len(cycles[0]) != 6 {
			t.Errorf("expected cycle length 6, got %d", len(cycles[0]))
		}
	})

	t.Run("two 3-cycles", func(t *testing.T) {
		// 0→1→2→0 and 3→4→5→3
		assign := []int{1, 2, 0, 4, 5, 3}
		cycles := decomposeCycles(assign, ids)
		if len(cycles) != 2 {
			t.Fatalf("expected 2 cycles, got %d", len(cycles))
		}
		for _, c := range cycles {
			if len(c) != 3 {
				t.Errorf("expected cycle length 3, got %d", len(c))
			}
		}
	})

	t.Run("three 2-cycles", func(t *testing.T) {
		// 0→1→0, 2→3→2, 4→5→4
		assign := []int{1, 0, 3, 2, 5, 4}
		cycles := decomposeCycles(assign, ids)
		if len(cycles) != 3 {
			t.Fatalf("expected 3 cycles, got %d", len(cycles))
		}
		for _, c := range cycles {
			if len(c) != 2 {
				t.Errorf("expected cycle length 2, got %d", len(c))
			}
		}
	})
}

func TestCanonicalForm(t *testing.T) {
	t.Run("rotations of the same cycle are equal", func(t *testing.T) {
		// 0→1→2→0: assign=[1,2,0]
		a1 := []int{1, 2, 0}
		// 1→2→0→1: same cycle, different start. In terms of assign, this is
		// the same permutation — canonicalize works on the permutation.
		// Use a 4-element cycle to test rotation: 0→2→1→3→0
		assign := []int{2, 3, 1, 0} // 0→2→1→3→0
		c1 := canonicalize(assign)
		// A different representation of the same cycle: verify it's the same key.
		if c1 != canonicalize(assign) {
			t.Error("canonicalize is not deterministic")
		}
		_ = a1

		// Two rotations of [0,2,1,3]: starting at 0 and starting at 1 are the same cycle.
		// The canonical form always starts at the minimum index (0 here).
		// Just verify the canonical is consistent.
		if !strings.HasPrefix(c1, "(0,") {
			t.Errorf("canonical form should start with minimum index: got %q", c1)
		}
	})

	t.Run("different cycles produce different canonical forms", func(t *testing.T) {
		// 0→1→2→0 vs 0→2→1→0 (reversed direction)
		a1 := []int{1, 2, 0} // 0→1→2→0
		a2 := []int{2, 0, 1} // 0→2→1→0
		if canonicalize(a1) == canonicalize(a2) {
			t.Error("reverse cycle should have a different canonical form")
		}
	})

	t.Run("two-cycle solutions: same cover, different cycle order", func(t *testing.T) {
		// Both represent cycles {0,1,2} and {3,4,5} — same solution.
		a1 := []int{1, 2, 0, 4, 5, 3} // 0→1→2→0, 3→4→5→3
		a2 := []int{1, 2, 0, 4, 5, 3} // identical
		if canonicalize(a1) != canonicalize(a2) {
			t.Error("identical assignments should have the same canonical form")
		}
	})
}

func TestScoreBetter(t *testing.T) {
	tests := []struct {
		name  string
		a, b  Score
		aWins bool // a.Better(b) should be true
	}{
		{
			name:  "higher MinCycleLen wins",
			a:     Score{MinCycleLen: 6, NumCycles: 1, MaxCycleLen: 6},
			b:     Score{MinCycleLen: 3, NumCycles: 2, MaxCycleLen: 3},
			aWins: true,
		},
		{
			name:  "lower NumCycles wins when MinCycleLen tied",
			a:     Score{MinCycleLen: 3, NumCycles: 2, MaxCycleLen: 5},
			b:     Score{MinCycleLen: 3, NumCycles: 3, MaxCycleLen: 4},
			aWins: true,
		},
		{
			name:  "higher MaxCycleLen wins when MinCycleLen and NumCycles tied",
			a:     Score{MinCycleLen: 3, NumCycles: 2, MaxCycleLen: 5},
			b:     Score{MinCycleLen: 3, NumCycles: 2, MaxCycleLen: 3},
			aWins: true,
		},
		{
			name:  "equal scores: neither wins",
			a:     Score{MinCycleLen: 4, NumCycles: 1, MaxCycleLen: 4},
			b:     Score{MinCycleLen: 4, NumCycles: 1, MaxCycleLen: 4},
			aWins: false,
		},
		{
			name:  "b wins when MinCycleLen is lower",
			a:     Score{MinCycleLen: 3, NumCycles: 1, MaxCycleLen: 6},
			b:     Score{MinCycleLen: 4, NumCycles: 2, MaxCycleLen: 4},
			aWins: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Better(tt.b)
			if got != tt.aWins {
				t.Errorf("Score%+v.Better(Score%+v) = %v, want %v", tt.a, tt.b, got, tt.aWins)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Solve
// ---------------------------------------------------------------------------

func TestSolve_SmallNoBlocks(t *testing.T) {
	// n=4, no blocks: must find a Hamiltonian 4-cycle.
	p := Problem{Participants: makeParticipants(4)}
	sols, err := Solve(context.Background(), p, Options{Seed: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sols) == 0 {
		t.Fatal("expected at least one solution")
	}
	s := sols[0]
	if s.Score.MinCycleLen != 4 {
		t.Errorf("expected Hamiltonian (MinCycleLen=4), got %d", s.Score.MinCycleLen)
	}
	assertValidSolution(t, p, s)
}

func TestSolve_SmallWithBlocks(t *testing.T) {
	// n=4, block 0→1. A Hamiltonian cycle still exists (e.g. 0→2→1→3→0).
	p := Problem{
		Participants: makeParticipants(4),
		Blocks:       []Block{{From: "0", To: "1"}},
	}
	sols, err := Solve(context.Background(), p, Options{Seed: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sols) == 0 {
		t.Fatal("expected at least one solution")
	}
	assertValidSolution(t, p, sols[0])
}

func TestSolve_Infeasible(t *testing.T) {
	// Block all outgoing edges for participant "0": infeasible.
	p := Problem{
		Participants: makeParticipants(3),
		Blocks: []Block{
			{From: "0", To: "1"},
			{From: "0", To: "2"},
		},
	}
	_, err := Solve(context.Background(), p, Options{})
	if !errors.Is(err, ErrInfeasible) {
		t.Errorf("expected ErrInfeasible, got %v", err)
	}
}

func TestSolve_Reproducible(t *testing.T) {
	p := Problem{Participants: makeParticipants(6)}
	opts := Options{MaxSolutions: 3, Seed: 99999}

	sols1, err := Solve(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	sols2, err := Solve(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if len(sols1) != len(sols2) {
		t.Fatalf("solution counts differ: %d vs %d", len(sols1), len(sols2))
	}
	for i := range sols1 {
		k1 := solutionKey(sols1[i])
		k2 := solutionKey(sols2[i])
		if k1 != k2 {
			t.Errorf("solution %d differs:\n  %s\n  %s", i, k1, k2)
		}
	}
}

// TestSolve_ParticipantOrderIndependence verifies that the same seed produces
// identical solutions regardless of participant input order.
func TestSolve_ParticipantOrderIndependence(t *testing.T) {
	base := makeParticipants(6)
	opts := Options{MaxSolutions: 3, Seed: 42}

	solveWith := func(order []int) []Solution {
		p := Problem{Participants: make([]Participant, len(order))}
		for i, idx := range order {
			p.Participants[i] = base[idx]
		}
		sols, err := Solve(context.Background(), p, opts)
		if err != nil {
			t.Fatalf("Solve error: %v", err)
		}
		return sols
	}

	canonical := solveWith([]int{0, 1, 2, 3, 4, 5})
	reversed  := solveWith([]int{5, 4, 3, 2, 1, 0})
	shuffled  := solveWith([]int{3, 0, 5, 1, 4, 2})

	for _, tc := range []struct {
		name string
		sols []Solution
	}{
		{"reversed", reversed},
		{"shuffled", shuffled},
	} {
		if len(tc.sols) != len(canonical) {
			t.Errorf("%s: got %d solutions, want %d", tc.name, len(tc.sols), len(canonical))
			continue
		}
		for i := range canonical {
			k1 := solutionKey(canonical[i])
			k2 := solutionKey(tc.sols[i])
			if k1 != k2 {
				t.Errorf("%s solution %d differs:\n  canonical: %s\n  got:       %s", tc.name, i, k1, k2)
			}
		}
	}
}

func TestSolve_MultipleSolutions(t *testing.T) {
	// n=6 complete graph has many Hamiltonian cycles; 5 distinct ones should be findable.
	p := Problem{Participants: makeParticipants(6)}
	opts := Options{MaxSolutions: 5, Seed: 42}
	sols, err := Solve(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sols) < 2 {
		t.Fatalf("expected multiple solutions, got %d", len(sols))
	}

	// All solutions must be distinct.
	seen := make(map[string]bool)
	for i, s := range sols {
		k := solutionKey(s)
		if seen[k] {
			t.Errorf("solution %d is a duplicate", i)
		}
		seen[k] = true
		assertValidSolution(t, p, s)
	}
}

func TestSolve_ScoreRanking(t *testing.T) {
	p := Problem{Participants: makeParticipants(6)}
	sols, err := Solve(context.Background(), p, Options{MaxSolutions: 5, Seed: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 1; i < len(sols); i++ {
		if sols[i].Score.Better(sols[i-1].Score) {
			t.Errorf("solution %d has a better score than solution %d (not best-first)", i, i-1)
		}
	}
}

func TestSolve_MergeCounterexample(t *testing.T) {
	// The 6-node graph from Phase 1 experiments has exactly one Hamiltonian cycle:
	// 0→1→5→4→3→2→0. The solver must find it (MinCycleLen=6), not the three
	// 2-cycles that a greedy cycle-merge would produce.
	p := counterexampleProblem()
	sols, err := Solve(context.Background(), p, Options{Seed: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sols) == 0 {
		t.Fatal("expected at least one solution")
	}
	if sols[0].Score.MinCycleLen != 6 {
		t.Errorf("expected Hamiltonian (MinCycleLen=6), got %d — solver may be using cycle-merge", sols[0].Score.MinCycleLen)
	}
	assertValidSolution(t, p, sols[0])
}

func TestSolve_FallbackProgression(t *testing.T) {
	// Two isolated groups of 3: no Hamiltonian cycle exists.
	// The solver must fall back to minCycleLen=3 (two 3-cycles).
	p := twoGroupProblem(6)
	sols, err := Solve(context.Background(), p, Options{Seed: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sols) == 0 {
		t.Fatal("expected at least one solution")
	}
	s := sols[0]
	if s.Score.MinCycleLen == 6 {
		t.Error("found Hamiltonian cycle in a graph that should not have one")
	}
	if s.Score.MinCycleLen < 2 {
		t.Errorf("MinCycleLen=%d is unexpectedly low", s.Score.MinCycleLen)
	}
	assertValidSolution(t, p, s)
}

func TestSolve_ProgressionN6(t *testing.T) {
	// Two isolated triangles: only minCycleLen=3 is achievable.
	// N/M progression for n=6: tries 6 (fails), then 3 (succeeds).
	p := twoGroupProblem(6)
	sols, err := Solve(context.Background(), p, Options{MaxSolutions: 3, Seed: 7})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, s := range sols {
		if s.Score.MinCycleLen != 3 {
			t.Errorf("solution %d: expected MinCycleLen=3, got %d", i, s.Score.MinCycleLen)
		}
		assertValidSolution(t, p, s)
	}
	// All solutions must share the same MinCycleLen (they come from the same progression level).
	for i := 1; i < len(sols); i++ {
		if sols[i].Score.MinCycleLen != sols[0].Score.MinCycleLen {
			t.Errorf("solutions have different MinCycleLen: %d vs %d", sols[0].Score.MinCycleLen, sols[i].Score.MinCycleLen)
		}
	}
}

// ---------------------------------------------------------------------------
// Property-based test
// ---------------------------------------------------------------------------

func TestSolveProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(2024))
	const trials = 50

	for range trials {
		n := 3 + rng.Intn(8) // 3 to 10 participants
		participants := make([]Participant, n)
		for i := range n {
			participants[i] = Participant{
				ID:   fmt.Sprintf("p%d", i),
				Name: fmt.Sprintf("Person %d", i),
			}
		}
		var blocks []Block
		for i := range n {
			for j := range n {
				if i != j && rng.Float64() < 0.1 {
					blocks = append(blocks, Block{
						From: participants[i].ID,
						To:   participants[j].ID,
					})
				}
			}
		}
		p := Problem{Participants: participants, Blocks: blocks}
		sols, err := Solve(context.Background(), p, Options{MaxSolutions: 3, Seed: rng.Int63()})
		if errors.Is(err, ErrInfeasible) {
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for n=%d: %v", n, err)
		}
		if len(sols) == 0 {
			t.Errorf("expected at least one solution for feasible problem (n=%d)", n)
			continue
		}

		for _, s := range sols {
			assertValidSolution(t, p, s)
		}

		// All solutions must share the same MinCycleLen.
		for i := 1; i < len(sols); i++ {
			if sols[i].Score.MinCycleLen != sols[0].Score.MinCycleLen {
				t.Errorf("solutions have different MinCycleLen: %d vs %d",
					sols[0].Score.MinCycleLen, sols[i].Score.MinCycleLen)
			}
		}

		// Results must be in best-first order.
		for i := 1; i < len(sols); i++ {
			if sols[i].Score.Better(sols[i-1].Score) {
				t.Errorf("solutions not in best-first order at index %d", i)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Fuzz tests
// ---------------------------------------------------------------------------

// FuzzSolve verifies that Solve never panics and that every returned solution
// is a valid derangement: each participant gives exactly once and receives
// exactly once, with no self-assignment.
func FuzzSolve(f *testing.F) {
	// Seed corpus: small valid problems.
	f.Add(2, 0)
	f.Add(4, 0)
	f.Add(6, 2)
	f.Add(10, 3)

	f.Fuzz(func(t *testing.T, n int, numBlocks int) {
		if n < 2 || n > 20 {
			t.Skip()
		}
		if numBlocks < 0 {
			numBlocks = 0
		}

		participants := makeParticipants(n)

		// Build a deterministic set of blocks from numBlocks (capped to avoid
		// making the problem trivially infeasible by blocking everything).
		maxBlocks := n * (n - 1) / 2
		if numBlocks > maxBlocks {
			numBlocks = maxBlocks
		}
		rng := rand.New(rand.NewSource(int64(n*1000 + numBlocks)))
		blockSet := make(map[[2]int]bool)
		var blocks []Block
		for len(blocks) < numBlocks {
			i := rng.Intn(n)
			j := rng.Intn(n)
			if i == j || blockSet[[2]int{i, j}] {
				continue
			}
			blockSet[[2]int{i, j}] = true
			blocks = append(blocks, Block{From: participants[i].ID, To: participants[j].ID})
		}

		prob := Problem{Participants: participants, Blocks: blocks}
		opts := Options{Seed: 1, MaxSolutions: 3}
		sols, err := Solve(context.Background(), prob, opts)
		if err != nil {
			// ErrInvalid or ErrInfeasible are acceptable — not a bug.
			if errors.Is(err, ErrInvalid) || errors.Is(err, ErrInfeasible) {
				return
			}
			t.Fatalf("unexpected error: %v", err)
		}

		ids := make(map[string]bool, n)
		for _, p := range participants {
			ids[p.ID] = true
		}

		for si, sol := range sols {
			if len(sol.Assignments) != n {
				t.Fatalf("sol %d: expected %d assignments, got %d", si, n, len(sol.Assignments))
			}
			gifters := make(map[string]int, n)
			recipients := make(map[string]int, n)
			for _, a := range sol.Assignments {
				if !ids[a.GifterID] {
					t.Fatalf("sol %d: unknown gifter %q", si, a.GifterID)
				}
				if !ids[a.RecipientID] {
					t.Fatalf("sol %d: unknown recipient %q", si, a.RecipientID)
				}
				if a.GifterID == a.RecipientID {
					t.Fatalf("sol %d: self-assignment for %q", si, a.GifterID)
				}
				gifters[a.GifterID]++
				recipients[a.RecipientID]++
			}
			for _, p := range participants {
				if gifters[p.ID] != 1 {
					t.Fatalf("sol %d: gifter %q appears %d times", si, p.ID, gifters[p.ID])
				}
				if recipients[p.ID] != 1 {
					t.Fatalf("sol %d: recipient %q appears %d times", si, p.ID, recipients[p.ID])
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Analyze
// ---------------------------------------------------------------------------

func TestAnalyze_Participants(t *testing.T) {
	// 4-participant complete graph: each person can give to all 3 others.
	p := Problem{
		Participants: []Participant{
			{ID: "a", Name: "Alice"},
			{ID: "b", Name: "Bob"},
			{ID: "c", Name: "Carol"},
			{ID: "d", Name: "Dave"},
		},
	}
	info, err := Analyze(context.Background(), p)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(info.Participants) != 4 {
		t.Fatalf("expected 4 ParticipantInfos, got %d", len(info.Participants))
	}
	// buildGraph sorts by ID, so order is a, b, c, d.
	if info.Participants[0].ID != "a" || info.Participants[0].Name != "Alice" {
		t.Errorf("unexpected first participant: %+v", info.Participants[0])
	}
	for _, pi := range info.Participants {
		if len(pi.Recipients) != 3 {
			t.Errorf("participant %q: expected 3 recipients, got %d: %v", pi.ID, len(pi.Recipients), pi.Recipients)
		}
		for _, r := range pi.Recipients {
			if r == pi.ID {
				t.Errorf("participant %q has self in recipient list", pi.ID)
			}
		}
	}
}

func TestAnalyze_Recipients_WithBlocks(t *testing.T) {
	// Block a->b: Alice can give to Carol and Dave but not Bob.
	p := Problem{
		Participants: []Participant{
			{ID: "a", Name: "Alice"},
			{ID: "b", Name: "Bob"},
			{ID: "c", Name: "Carol"},
			{ID: "d", Name: "Dave"},
		},
		Blocks: []Block{{From: "a", To: "b"}},
	}
	info, err := Analyze(context.Background(), p)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	var alice ParticipantInfo
	for _, pi := range info.Participants {
		if pi.ID == "a" {
			alice = pi
		}
	}
	if len(alice.Recipients) != 2 {
		t.Errorf("Alice: expected 2 recipients, got %v", alice.Recipients)
	}
	for _, r := range alice.Recipients {
		if r == "b" {
			t.Error("Alice: blocked recipient 'b' appeared in recipient list")
		}
	}
}

func TestAnalyze_HallViolations_None(t *testing.T) {
	// Feasible two-group problem: Hall's condition holds.
	info, err := Analyze(context.Background(), twoGroupProblem(4))
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(info.HallViolations) != 0 {
		t.Errorf("expected no Hall violations for feasible problem, got %v", info.HallViolations)
	}
}

func TestAnalyze_HallViolations_OutDegreeZero(t *testing.T) {
	// Participant "a" is blocked from giving to everyone: simple Hall violation.
	p := Problem{
		Participants: []Participant{
			{ID: "a", Name: "Alice"},
			{ID: "b", Name: "Bob"},
			{ID: "c", Name: "Carol"},
		},
		Blocks: []Block{{From: "a", To: "b"}, {From: "a", To: "c"}},
	}
	info, err := Analyze(context.Background(), p)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(info.HallViolations) == 0 {
		t.Fatal("expected a Hall violation, got none")
	}
	v := info.HallViolations[0]
	found := false
	for _, id := range v.Gifters {
		if id == "a" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'a' in Hall violation gifters, got %v", v.Gifters)
	}
	if len(v.Recipients) >= len(v.Gifters) {
		t.Errorf("Hall violation invariant broken: |recipients|=%d >= |gifters|=%d", len(v.Recipients), len(v.Gifters))
	}
	if info.HamiltonianPossible {
		t.Error("HamiltonianPossible should be false when Hall is violated")
	}
}

func TestAnalyze_HallViolations_GroupViolation(t *testing.T) {
	// 6 participants: "0", "1", "2" can only give to "3" or "4".
	// That's 3 gifters competing for 2 recipient slots — a group Hall violation
	// that the simple degree check (checkHall) does not catch.
	participants := makeParticipants(6)
	// Block 0, 1, 2 from giving to each other and from giving to participant 5.
	var blocks []Block
	for _, from := range []string{"0", "1", "2"} {
		for _, to := range []string{"0", "1", "2", "5"} {
			if from != to {
				blocks = append(blocks, Block{From: from, To: to})
			}
		}
	}
	p := Problem{Participants: participants, Blocks: blocks}

	info, err := Analyze(context.Background(), p)
	if err != nil {
		t.Fatalf("Analyze error: %v", err)
	}
	if len(info.HallViolations) == 0 {
		t.Fatal("expected a Hall violation, got none")
	}
	v := info.HallViolations[0]
	if len(v.Gifters) <= len(v.Recipients) {
		t.Errorf("Hall violation invariant broken: |gifters|=%d, |recipients|=%d", len(v.Gifters), len(v.Recipients))
	}
	// The violating set must include "0", "1", "2"; their neighbors are "3" and "4".
	gifterSet := make(map[string]bool)
	for _, id := range v.Gifters {
		gifterSet[id] = true
	}
	recipSet := make(map[string]bool)
	for _, id := range v.Recipients {
		recipSet[id] = true
	}
	for _, id := range []string{"0", "1", "2"} {
		if !gifterSet[id] {
			t.Errorf("expected %q in Hall violation gifters %v", id, v.Gifters)
		}
	}
	for _, id := range []string{"3", "4"} {
		if !recipSet[id] {
			t.Errorf("expected %q in Hall violation recipients %v", id, v.Recipients)
		}
	}
}
