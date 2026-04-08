package giftexchange

import (
	"context"
	"math/rand"
)

// Analyze returns graph statistics for the given problem, including per-participant
// recipient lists, Hall condition diagnostics, and whether a Hamiltonian cycle is
// possible. The Hamiltonian check runs a full DFS and may be slow for large,
// heavily-constrained graphs; cancel ctx to abort it.
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

	// Build per-participant recipient info. buildGraph sorts participants by ID,
	// so g.ids and g.adj are already in sorted-ID order.
	nameOf := make(map[string]string, len(p.Participants))
	for _, part := range p.Participants {
		nameOf[part.ID] = part.Name
	}
	participants := make([]ParticipantInfo, n)
	for i := range n {
		recips := make([]string, len(g.adj[i]))
		for k, j := range g.adj[i] {
			recips[k] = g.ids[j]
		}
		participants[i] = ParticipantInfo{
			ID:         g.ids[i],
			Name:       nameOf[g.ids[i]],
			Recipients: recips,
		}
	}

	// Hall condition check via max bipartite matching. If a Hall violation exists,
	// a Hamiltonian cycle is impossible and the DFS can be skipped.
	violations := hallViolations(g)

	hamiltonian := false
	if len(violations) == 0 {
		// hamiltonianSolver checks ctx internally (every 256 calls).
		rng := rand.New(rand.NewSource(0))
		_, hamiltonian = hamiltonianSolver(ctx, g, rng)
		if ctx.Err() != nil {
			return GraphInfo{}, ctx.Err()
		}
	}

	return GraphInfo{
		ParticipantCount:    n,
		EdgeCount:           edgeCount,
		MaxEdgeCount:        maxEdgeCount,
		Density:             density,
		HamiltonianPossible: hamiltonian,
		Participants:        participants,
		HallViolations:      violations,
	}, nil
}

// hallViolations finds whether a perfect bipartite matching exists for the
// given graph. If not, it returns a Hall witness: a subset S of gifters whose
// collective valid recipient set N(S) has |N(S)| < |S|.
//
// Algorithm: augmenting-path max bipartite matching (O(n^3) worst case),
// followed by an alternating-tree BFS from all unmatched gifters to extract
// the violating subset.
func hallViolations(g *graph) []HallViolation {
	// matchLeft[i] = recipient index matched to gifter i, or -1.
	// matchRight[j] = gifter index matched to recipient j, or -1.
	matchLeft := make([]int, g.n)
	matchRight := make([]int, g.n)
	for i := range g.n {
		matchLeft[i] = -1
		matchRight[i] = -1
	}

	var augment func(gifter int, visited []bool) bool
	augment = func(gifter int, visited []bool) bool {
		for _, r := range g.adj[gifter] {
			if visited[r] {
				continue
			}
			visited[r] = true
			if matchRight[r] == -1 || augment(matchRight[r], visited) {
				matchLeft[gifter] = r
				matchRight[r] = gifter
				return true
			}
		}
		return false
	}

	for i := range g.n {
		visited := make([]bool, g.n)
		augment(i, visited)
	}

	// Collect unmatched gifters; if none, the matching is perfect — no violation.
	var unmatched []int
	for i := range g.n {
		if matchLeft[i] == -1 {
			unmatched = append(unmatched, i)
		}
	}
	if len(unmatched) == 0 {
		return nil
	}

	// BFS alternating tree from all unmatched gifters simultaneously.
	// A free edge reaches a recipient; the matched edge from that recipient
	// reaches the gifter currently holding it, who is then explored further.
	// The reachable gifter set S and reachable recipient set T = N(S) satisfy
	// |S| > |T|, which is the Hall violation witness.
	reachableGifter := make([]bool, g.n)
	reachableRecip := make([]bool, g.n)
	queue := make([]int, 0, g.n)
	for _, u := range unmatched {
		reachableGifter[u] = true
		queue = append(queue, u)
	}
	for len(queue) > 0 {
		gifter := queue[0]
		queue = queue[1:]
		for _, r := range g.adj[gifter] {
			if reachableRecip[r] {
				continue
			}
			reachableRecip[r] = true
			g2 := matchRight[r]
			if g2 != -1 && !reachableGifter[g2] {
				reachableGifter[g2] = true
				queue = append(queue, g2)
			}
		}
	}

	// IDs are in sorted order since buildGraph sorts participants by ID.
	var gifterIDs, recipIDs []string
	for i := range g.n {
		if reachableGifter[i] {
			gifterIDs = append(gifterIDs, g.ids[i])
		}
	}
	for j := range g.n {
		if reachableRecip[j] {
			recipIDs = append(recipIDs, g.ids[j])
		}
	}
	return []HallViolation{{Gifters: gifterIDs, Recipients: recipIDs}}
}
