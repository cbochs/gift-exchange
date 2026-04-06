package giftexchange

import (
	"slices"
	"strconv"
	"strings"
)

// decomposeCycles decomposes an assignment into its constituent directed cycles.
// assign[i] = j means participant i gives to participant j.
func decomposeCycles(assign []int, ids []string) []Cycle {
	visited := make([]bool, len(assign))
	var cycles []Cycle
	for start := range len(assign) {
		if visited[start] {
			continue
		}
		var cycle Cycle
		cur := start
		for !visited[cur] {
			visited[cur] = true
			cycle = append(cycle, ids[cur])
			cur = assign[cur]
		}
		cycles = append(cycles, cycle)
	}
	return cycles
}

// canonicalize produces a canonical string for an assignment, used to detect
// duplicate solutions. Two assignments that represent the same cycle cover
// (accounting for cycle rotation) produce identical canonical strings.
func canonicalize(assign []int) string {
	n := len(assign)
	visited := make([]bool, n)
	var cycleStrs []string
	for start := range n {
		if visited[start] {
			continue
		}
		var indices []int
		cur := start
		for !visited[cur] {
			visited[cur] = true
			indices = append(indices, cur)
			cur = assign[cur]
		}
		// Rotate so the smallest index is first.
		minPos := 0
		for i := 1; i < len(indices); i++ {
			if indices[i] < indices[minPos] {
				minPos = i
			}
		}
		parts := make([]string, len(indices))
		for i := range indices {
			parts[i] = strconv.Itoa(indices[(minPos+i)%len(indices)])
		}
		cycleStrs = append(cycleStrs, "("+strings.Join(parts, ",")+")")
	}
	slices.Sort(cycleStrs)
	return strings.Join(cycleStrs, "|")
}

// scoreOf computes the Score for a set of cycles.
func scoreOf(cycles []Cycle) Score {
	if len(cycles) == 0 {
		return Score{}
	}
	minLen := len(cycles[0])
	maxLen := len(cycles[0])
	for _, c := range cycles[1:] {
		if len(c) < minLen {
			minLen = len(c)
		}
		if len(c) > maxLen {
			maxLen = len(c)
		}
	}
	return Score{
		MinCycleLen: minLen,
		NumCycles:   len(cycles),
		MaxCycleLen: maxLen,
	}
}

// Better reports whether s is strictly better than other.
// Rankings: maximize MinCycleLen, then minimize NumCycles, then maximize MaxCycleLen.
func (s Score) Better(other Score) bool {
	if s.MinCycleLen != other.MinCycleLen {
		return s.MinCycleLen > other.MinCycleLen
	}
	if s.NumCycles != other.NumCycles {
		return s.NumCycles < other.NumCycles
	}
	return s.MaxCycleLen > other.MaxCycleLen
}

func sortByScore(solutions []Solution) {
	slices.SortFunc(solutions, func(a, b Solution) int {
		if a.Score.Better(b.Score) {
			return -1
		}
		if b.Score.Better(a.Score) {
			return 1
		}
		return 0
	})
}

func makeSolution(assign []int, g *graph) Solution {
	cycles := decomposeCycles(assign, g.ids)
	score := scoreOf(cycles)
	assignments := make([]Assignment, len(assign))
	for i, j := range assign {
		assignments[i] = Assignment{
			GifterID:    g.ids[i],
			RecipientID: g.ids[j],
		}
	}
	return Solution{
		Assignments: assignments,
		Cycles:      cycles,
		Score:       score,
	}
}
