package giftexchange

import (
	"context"
)

// Analyze returns graph statistics for the given problem, including per-participant
// recipient lists, Hall condition diagnostics, and dead edge analysis.
// The dead edge analysis may be slow for large, heavily-constrained graphs;
// cancel ctx to abort it.
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

	// Hall condition check via max bipartite matching.
	violations := hallViolations(g)

	// Dead edge analysis is skipped when Hall violations exist.
	var solutionDead, hamiltonianDead []DeadEdge
	if len(violations) == 0 {
		for i := range n {
			for _, j := range g.adj[i] {
				if ctx.Err() != nil {
					return GraphInfo{}, ctx.Err()
				}
				if !canCompleteMatching(g, i, j) {
					solutionDead = append(solutionDead, DeadEdge{
						Gifter:    g.ids[i],
						Recipient: g.ids[j],
					})
				} else if !hamiltonianPathExists(ctx, g, j, i) {
					if ctx.Err() != nil {
						return GraphInfo{}, ctx.Err()
					}
					hamiltonianDead = append(hamiltonianDead, DeadEdge{
						Gifter:    g.ids[i],
						Recipient: g.ids[j],
					})
				}
			}
		}
	}

	return GraphInfo{
		ParticipantCount:     n,
		EdgeCount:            edgeCount,
		MaxEdgeCount:         maxEdgeCount,
		Density:              density,
		Participants:         participants,
		HallViolations:       violations,
		SolutionDeadEdges:    solutionDead,
		HamiltonianDeadEdges: hamiltonianDead,
	}, nil
}

// canCompleteMatching reports whether the remaining n-1 participants can be
// bipartite-matched after fixing gifter excludeGifter → recipient excludeRecipient.
func canCompleteMatching(g *graph, excludeGifter, excludeRecipient int) bool {
	matchLeft := make([]int, g.n)
	matchRight := make([]int, g.n)
	for i := range g.n {
		matchLeft[i] = -1
		matchRight[i] = -1
	}

	var augment func(gifter int, visited []bool) bool
	augment = func(gifter int, visited []bool) bool {
		for _, r := range g.adj[gifter] {
			if r == excludeRecipient || visited[r] {
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

	matched := 0
	for i := range g.n {
		if i == excludeGifter {
			continue
		}
		visited := make([]bool, g.n)
		visited[excludeRecipient] = true // mark excluded recipient as seen so it's skipped
		if augment(i, visited) {
			matched++
		}
	}
	return matched == g.n-1
}

// hamiltonianPathExists reports whether a Hamiltonian path from start to end
// exists, visiting all n participants exactly once.
// end is reserved: it may not be visited as an intermediate node.
func hamiltonianPathExists(ctx context.Context, g *graph, start, end int) bool {
	visited := make([]bool, g.n)
	visited[start] = true
	calls := 0

	var dfs func(cur, depth int) bool
	dfs = func(cur, depth int) bool {
		calls++
		if calls&0xFF == 0 && ctx.Err() != nil {
			return false
		}
		if depth == g.n-1 {
			// All nodes visited except end; check if end is reachable.
			return g.isEdge(cur, end)
		}
		for _, next := range g.adj[cur] {
			if visited[next] || next == end {
				continue
			}
			visited[next] = true
			if dfs(next, depth+1) {
				return true
			}
			visited[next] = false
		}
		return false
	}

	return dfs(start, 1)
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
