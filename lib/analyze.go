package giftexchange

import (
	"context"
	"math/rand"
)


// Analyze returns graph statistics for the given problem, including whether
// a Hamiltonian cycle is possible. The Hamiltonian check runs a full DFS and
// may be slow for large, heavily-constrained graphs; cancel ctx to abort it.
func Analyze(ctx context.Context, p Problem) (GraphInfo, error) {
	if err := validateStructural(p); err != nil {
		return GraphInfo{}, err
	}
	g := buildGraph(p.Participants, p.Blocks)
	n := g.n

	edgeCount := 0
	for i := range n {
		edgeCount += len(g.adj[i])
	}
	maxEdgeCount := n * (n - 1)
	var density float64
	if maxEdgeCount > 0 {
		density = float64(edgeCount) / float64(maxEdgeCount)
	}

	// hamiltonianSolver checks ctx internally (every 256 calls), so calling it
	// directly is sufficient — no goroutine needed.
	rng := rand.New(rand.NewSource(0))
	_, hamiltonian := hamiltonianSolver(ctx, g, rng)
	if ctx.Err() != nil {
		return GraphInfo{}, ctx.Err()
	}

	return GraphInfo{
		ParticipantCount:    n,
		EdgeCount:           edgeCount,
		MaxEdgeCount:        maxEdgeCount,
		Density:             density,
		HamiltonianPossible: hamiltonian,
	}, nil
}
