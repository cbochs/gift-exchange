package giftexchange

import (
	"math/rand"
	"slices"
)

type graph struct {
	n   int
	ids []string       // ids[i] = participant ID at index i
	idx map[string]int // idx[id] = index
	adj [][]int        // adj[i] = sorted list of valid recipient indices for gifter i
}

func buildGraph(participants []Participant, blocks []Block) *graph {
	// Sort by ID so the node→index mapping is independent of input order.
	// This ensures a given seed always produces the same result regardless of
	// the order participants were added.
	participants = slices.Clone(participants)
	slices.SortFunc(participants, func(a, b Participant) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	n := len(participants)
	g := &graph{
		n:   n,
		ids: make([]string, n),
		idx: make(map[string]int),
		adj: make([][]int, n),
	}
	for i, p := range participants {
		g.ids[i] = p.ID
		g.idx[p.ID] = i
	}

	blocked := make(map[[2]int]bool)
	for _, b := range blocks {
		fi := g.idx[b.From]
		ti := g.idx[b.To]
		blocked[[2]int{fi, ti}] = true
	}

	for i := range n {
		for j := range n {
			if i != j && !blocked[[2]int{i, j}] {
				g.adj[i] = append(g.adj[i], j)
			}
		}
	}
	return g
}

func (g *graph) isEdge(from, to int) bool {
	return slices.Contains(g.adj[from], to)
}

// shuffled returns a shuffled copy of adj using rng, leaving the original unchanged.
func shuffled(adj []int, rng *rand.Rand) []int {
	out := make([]int, len(adj))
	copy(out, adj)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}
