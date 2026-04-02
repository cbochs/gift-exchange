package giftexchange

import (
	"context"
	"math/rand"
)

// Analyze returns graph statistics for the given problem, including whether
// a Hamiltonian cycle is possible. The Hamiltonian check runs a full DFS and
// may be slow for large, heavily-constrained graphs; cancel ctx to abort it.
func Analyze(ctx context.Context, p Problem) (GraphInfo, error) {
	if err := validate(p); err != nil {
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

	// Run the Hamiltonian DFS in a goroutine so context cancellation is respected.
	type dfsResult struct{ ok bool }
	ch := make(chan dfsResult, 1)
	go func() {
		rng := rand.New(rand.NewSource(0))
		_, ok := hamiltonianDFS(g, rng)
		ch <- dfsResult{ok}
	}()

	select {
	case r := <-ch:
		return GraphInfo{
			ParticipantCount:    n,
			EdgeCount:           edgeCount,
			MaxEdgeCount:        maxEdgeCount,
			Density:             density,
			HamiltonianPossible: r.ok,
		}, nil
	case <-ctx.Done():
		return GraphInfo{}, ctx.Err()
	}
}
