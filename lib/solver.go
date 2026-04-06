package giftexchange

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// Solve is the single entry point for the library.
// Returns up to opts.MaxSolutions solutions ranked best-first.
// Returns ErrInfeasible if no valid assignment exists under any constraint level.
func Solve(ctx context.Context, p Problem, opts Options) ([]Solution, error) {
	if err := validateStructural(p); err != nil {
		return nil, err
	}

	if opts.MaxSolutions <= 0 {
		opts.MaxSolutions = DefaultMaxSolutions
	}

	seed := opts.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	g := buildGraph(p.Participants, p.Blocks)
	if err := checkHall(g); err != nil {
		return nil, err
	}
	n := g.n

	// N/M progression: try minCycleLen = N, N/2, N/3, ... until a solution is
	// found or the target drops to 1 (infeasible under all cycle-length constraints).
	lastTarget := -1
	for M := 1; ; M++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		target := n / M
		if target <= 1 {
			return nil, ErrInfeasible
		}
		if target == lastTarget {
			continue // integer division produced a duplicate target; skip
		}
		lastTarget = target

		var solver solverFunc
		if target == n {
			solver = hamiltonianSolver
		} else {
			solver = constrainedSolver(target)
		}
		solutions := collectSolutions(ctx, solver, g, seed, opts.MaxSolutions)
		if len(solutions) > 0 {
			sortByScore(solutions)
			return solutions, nil
		}
	}
}

// Validate checks the problem for structural errors and Hall's condition.
// Returns nil if the problem is well-formed and potentially solvable.
// This is a thin exported wrapper for callers (e.g. the CLI validate subcommand)
// that need to check feasibility without running the full solver.
func Validate(p Problem) error {
	if err := validateStructural(p); err != nil {
		return err
	}
	return checkHall(buildGraph(p.Participants, p.Blocks))
}

// validateStructural checks participant count, duplicate IDs, and block
// participant references. Returns ErrInvalid (wrapped) on failure.
// No graph is built; this is safe to call before buildGraph.
func validateStructural(p Problem) error {
	if len(p.Participants) < 2 {
		return fmt.Errorf("%w: at least 2 participants required, got %d", ErrInvalid, len(p.Participants))
	}

	ids := make(map[string]bool, len(p.Participants))
	for _, part := range p.Participants {
		if ids[part.ID] {
			return fmt.Errorf("%w: duplicate participant ID: %q", ErrInvalid, part.ID)
		}
		ids[part.ID] = true
	}

	for _, b := range p.Blocks {
		if !ids[b.From] {
			return fmt.Errorf("%w: block references unknown participant ID: %q", ErrInvalid, b.From)
		}
		if !ids[b.To] {
			return fmt.Errorf("%w: block references unknown participant ID: %q", ErrInvalid, b.To)
		}
	}
	return nil
}

// checkHall verifies Hall's condition: every participant has at least one
// valid recipient (out-degree ≥ 1) and at least one valid gifter (in-degree ≥ 1).
// Returns ErrInfeasible if the condition is violated.
func checkHall(g *graph) error {
	inDegree := make([]int, g.n)
	for i := range g.n {
		if len(g.adj[i]) == 0 {
			return ErrInfeasible
		}
		for _, j := range g.adj[i] {
			inDegree[j]++
		}
	}
	for i := range g.n {
		if inDegree[i] == 0 {
			return ErrInfeasible
		}
	}
	return nil
}

// solverFunc attempts to find a valid assignment for the given graph.
// Returns (assign, true) on success, (nil, false) if no solution exists at
// this constraint level. Must respect ctx cancellation.
type solverFunc func(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool)

// hamiltonianSolver attempts to find a Hamiltonian cycle in g using depth-first
// search starting from node 0 (cycle rotation-invariance means fixing the start
// does not miss any solutions). Per-node adjacency list shuffling ensures
// different calls explore different paths, producing diverse solutions.
//
// Returns (assign, true) on success, (nil, false) if no Hamiltonian cycle exists
// or ctx is canceled.
func hamiltonianSolver(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool) {
	assign := make([]int, g.n)
	for i := range assign {
		assign[i] = -1
	}
	visited := make([]bool, g.n)
	path := make([]int, 0, g.n)
	path = append(path, 0)
	visited[0] = true

	var calls int
	var dfs func() bool
	dfs = func() bool {
		// Check context every 256 calls — negligible overhead, enables clean cancellation.
		calls++
		if calls&0xFF == 0 && ctx.Err() != nil {
			return false
		}
		if len(path) == g.n {
			last := path[len(path)-1]
			if g.isEdge(last, 0) {
				for i := range len(path) - 1 {
					assign[path[i]] = path[i+1]
				}
				assign[last] = 0
				return true
			}
			return false
		}
		cur := path[len(path)-1]
		for _, next := range shuffled(g.adj[cur], rng) {
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

// wouldClosePrematureCycle reports whether assigning gifter→recipient would close
// a cycle shorter than minLen. Closing any cycle shorter than minLen must be
// rejected to ensure all cycles in the final solution meet the minimum length.
//
// Note: the plan's pseudocode uses `length < minLen || assigned < total`, but
// the textual description and correct logic is simply `length < minLen`. The `||`
// form would reject all intermediate cycle closures, preventing multi-cycle solutions.
func wouldClosePrematureCycle(assign []int, gifter, recipient, minLen int) bool {
	length := 1
	cur := recipient
	for {
		next := assign[cur]
		if next < 0 {
			return false // open chain — no cycle would form
		}
		length++
		if next == gifter {
			return length < minLen
		}
		cur = next
	}
}

// constrainedSolver returns a solverFunc that finds a valid cycle cover where
// all cycles have length >= minCycleLen. Used only when hamiltonianSolver has
// confirmed no Hamiltonian cycle exists (i.e., for M > 1 in the N/M progression).
func constrainedSolver(minCycleLen int) solverFunc {
	return func(ctx context.Context, g *graph, rng *rand.Rand) ([]int, bool) {
		assign := make([]int, g.n)
		for i := range assign {
			assign[i] = -1
		}
		usedRecipient := make([]bool, g.n)

		var calls int
		var backtrack func(gifter int) bool
		backtrack = func(gifter int) bool {
			// Check context every 256 calls.
			calls++
			if calls&0xFF == 0 && ctx.Err() != nil {
				return false
			}
			if gifter == g.n {
				return true
			}
			for _, recipient := range shuffled(g.adj[gifter], rng) {
				if usedRecipient[recipient] {
					continue
				}
				if wouldClosePrematureCycle(assign, gifter, recipient, minCycleLen) {
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

		if backtrack(0) {
			return assign, true
		}
		return nil, false
	}
}

// collectSolutions attempts to find up to max distinct valid assignments using
// solver. Stops early after collisionThreshold consecutive duplicate solutions
// (solution space near-exhausted).
//
// Returns nil if the solver reports the target is infeasible for this graph.
func collectSolutions(ctx context.Context, solver solverFunc, g *graph, seed int64, max int) []Solution {
	const collisionThreshold = 5

	seen := make(map[string]bool)
	var results []Solution
	consecutive := 0

	masterRNG := rand.New(rand.NewSource(seed))

	for len(results) < max {
		if ctx.Err() != nil {
			break
		}
		attemptSeed := masterRNG.Int63()
		rng := rand.New(rand.NewSource(attemptSeed))

		assign, ok := solver(ctx, g, rng)
		if !ok {
			// This target is infeasible for this graph — signal via empty results.
			return nil
		}

		canon := canonicalize(assign)
		if seen[canon] {
			consecutive++
			if consecutive >= collisionThreshold {
				break
			}
		} else {
			seen[canon] = true
			consecutive = 0
			results = append(results, makeSolution(assign, g))
		}
	}
	return results
}
