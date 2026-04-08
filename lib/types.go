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

// ParticipantInfo summarizes one participant's valid gifting options.
type ParticipantInfo struct {
	ID         string
	Name       string
	Recipients []string // participant IDs this participant can give to (sorted by ID)
}

// HallViolation identifies a subset S of gifters whose collective valid
// recipient set N(S) is smaller than S itself, making a complete assignment
// impossible. len(Recipients) < len(Gifters) always holds.
type HallViolation struct {
	Gifters    []string // participant IDs in the violating subset S (sorted)
	Recipients []string // participant IDs in N(S), the valid recipients of S (sorted)
}

// DeadEdge is a valid (non-blocked) directed edge that cannot be used in
// some or all solution types.
type DeadEdge struct {
	Gifter    string // participant ID
	Recipient string // participant ID
}

// GraphInfo contains statistics about the gift exchange constraint graph,
// returned by Analyze.
type GraphInfo struct {
	ParticipantCount int
	EdgeCount        int
	MaxEdgeCount     int     // n*(n-1), fully-connected directed graph
	Density          float64 // EdgeCount / MaxEdgeCount
	// Participants lists each participant's valid recipients, sorted by participant ID.
	Participants []ParticipantInfo
	// HallViolations is nil when Hall's condition holds (a perfect matching exists).
	// Otherwise it contains a witness subset S where |N(S)| < |S|.
	HallViolations []HallViolation
	// SolutionDeadEdges lists valid edges where fixing u→v makes any valid complete
	// assignment impossible. nil when Hall violations exist (analysis skipped).
	SolutionDeadEdges []DeadEdge
	// HamiltonianDeadEdges lists valid edges where fixing u→v still allows some
	// multi-cycle solution but rules out any Hamiltonian cycle. Excludes edges
	// already in SolutionDeadEdges. nil when Hall violations exist (skipped).
	HamiltonianDeadEdges []DeadEdge
}
