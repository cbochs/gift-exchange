package giftexchange

import (
	"errors"
	"time"
)

// DefaultMaxSolutions is the number of solutions Solve returns when
// Options.MaxSolutions is zero.
const DefaultMaxSolutions = 5

// NewSeed returns a non-deterministic seed suitable for Options.Seed.
func NewSeed() int64 { return time.Now().UnixNano() }

// ErrInvalid is returned when the Problem definition is structurally malformed:
// too few participants, duplicate IDs, or constraint references to unknown IDs.
// It is distinct from ErrInfeasible: an invalid problem has a definition error;
// an infeasible problem is well-formed but has no valid assignment.
var ErrInvalid = errors.New("invalid problem")

// ErrInfeasible is returned when no valid gift exchange assignment exists
// under the given constraints (all N/M progression levels exhausted, or
// Hall's condition violated).
var ErrInfeasible = errors.New("no valid gift exchange is possible under the given constraints")

// Participant is a person in the gift exchange.
type Participant struct {
	ID   string // unique identifier (used in blocks and output)
	Name string // display name
}

// Block is a directed constraint: From cannot give to To.
type Block struct {
	From string // this participant cannot give...
	To   string // ...to this participant (directed constraint)
}

// Problem is the input to Solve and Validate.
type Problem struct {
	Participants []Participant
	Blocks       []Block
	// No MinCycleLen: the solver automatically finds the best achievable
	// cycle structure via the N/M progression (N, N/2, N/3, ...).
}

// Options controls solver behavior.
type Options struct {
	MaxSolutions int           // max solutions to return (default: DefaultMaxSolutions)
	Seed         int64         // RNG seed; use NewSeed() for a random seed
	Timeout      time.Duration // max solver wall time; 0 = no limit
}

// Assignment is one gifter→recipient pair.
type Assignment struct {
	GifterID    string
	RecipientID string
}

// Cycle is a sequence of participant IDs: Cycle[0]→Cycle[1]→...→Cycle[0].
type Cycle []string

// Score captures solution quality metrics used for ranking.
type Score struct {
	MinCycleLen int // primary ranking: maximize
	NumCycles   int // secondary ranking: minimize
	MaxCycleLen int // tertiary ranking: maximize
}

// Solution is one valid gift exchange assignment with its cycle decomposition.
type Solution struct {
	Assignments []Assignment
	Cycles      []Cycle
	Score       Score
}

// GraphInfo contains statistics about the gift exchange constraint graph,
// returned by Analyze.
type GraphInfo struct {
	ParticipantCount    int
	EdgeCount           int
	MaxEdgeCount        int     // n*(n-1), fully-connected directed graph
	Density             float64 // EdgeCount / MaxEdgeCount
	HamiltonianPossible bool
}
