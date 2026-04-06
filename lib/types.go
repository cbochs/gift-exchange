package giftexchange

import (
	"errors"
	"time"
)

// ErrInvalid is returned when the Problem definition is structurally malformed:
// too few participants, duplicate IDs, or constraint references to unknown IDs.
// It is distinct from ErrInfeasible: an invalid problem has a definition error;
// an infeasible problem is well-formed but has no valid assignment.
var ErrInvalid = errors.New("invalid problem")

// ErrInfeasible is returned when no valid gift exchange assignment exists
// under the given constraints (all N/M progression levels exhausted, or
// Hall's condition violated).
var ErrInfeasible = errors.New("no valid gift exchange is possible under the given constraints")

type Participant struct {
	ID   string `json:"id"`   // unique identifier (used in blocks and output)
	Name string `json:"name"` // display name
}

type Block struct {
	From string `json:"from"` // this participant cannot give...
	To   string `json:"to"`   // ...to this participant (directed constraint)
}

type Problem struct {
	Participants []Participant
	Blocks       []Block
	// No MinCycleLen: the solver automatically finds the best achievable
	// cycle structure via the N/M progression (N, N/2, N/3, ...).
}

type Options struct {
	MaxSolutions int           // max solutions to return (default: 5)
	Seed         int64         // RNG seed; 0 = random (non-reproducible)
	Timeout      time.Duration // max solver wall time; 0 = no limit
}

type Assignment struct {
	GifterID    string `json:"gifter_id"`
	RecipientID string `json:"recipient_id"`
}

type Cycle []string // participant IDs in order: Cycle[0]→Cycle[1]→...→Cycle[0]

type Score struct {
	MinCycleLen int `json:"min_cycle_len"` // primary ranking: maximize
	NumCycles   int `json:"num_cycles"`    // secondary ranking: minimize
	MaxCycleLen int `json:"max_cycle_len"` // tertiary ranking: maximize
}

type Solution struct {
	Assignments []Assignment `json:"assignments"`
	Cycles      []Cycle      `json:"cycles"`
	Score       Score        `json:"score"`
}

// GraphInfo contains statistics about the gift exchange constraint graph,
// returned by Analyze.
type GraphInfo struct {
	ParticipantCount    int     `json:"participant_count"`
	EdgeCount           int     `json:"edge_count"`
	MaxEdgeCount        int     `json:"max_edge_count"` // n*(n-1), fully-connected directed graph
	Density             float64 `json:"density"`        // EdgeCount / MaxEdgeCount
	HamiltonianPossible bool    `json:"hamiltonian_possible"`
}
