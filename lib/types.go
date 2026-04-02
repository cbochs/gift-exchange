package giftexchange

import (
	"errors"
	"time"
)

// ErrInfeasible is returned when no valid gift exchange assignment exists
// under the given constraints (all N/M progression levels exhausted, or
// Hall's condition violated).
var ErrInfeasible = errors.New("no valid gift exchange is possible under the given constraints")

type Participant struct {
	ID   string // unique identifier (used in blocks and output)
	Name string // display name
}

type Block struct {
	From string // this participant cannot give...
	To   string // ...to this participant (directed constraint)
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
	GifterID    string
	RecipientID string
}

type Cycle []string // participant IDs in order: Cycle[0]→Cycle[1]→...→Cycle[0]

type Score struct {
	MinCycleLen int // primary ranking: maximize
	NumCycles   int // secondary ranking: minimize
	MaxCycleLen int // tertiary ranking: maximize
}

type Solution struct {
	Assignments []Assignment
	Cycles      []Cycle
	Score       Score
}
