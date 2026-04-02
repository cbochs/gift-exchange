package main

// ParticipantDTO is a participant in a gift exchange problem.
type ParticipantDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BlockDTO forbids a specific directed pairing.
type BlockDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// OptionsDTO controls solver behavior. All fields are optional.
// There is no min_cycle_len: the solver automatically finds the best achievable
// minimum cycle length via the N/M progression.
type OptionsDTO struct {
	MaxSolutions int   `json:"max_solutions,omitempty"`
	Seed         int64 `json:"seed,omitempty"`
	TimeoutMs    int   `json:"timeout_ms,omitempty"`
}

// SolveRequest is the body of POST /api/v1/solve.
type SolveRequest struct {
	Participants []ParticipantDTO `json:"participants"`
	Blocks       []BlockDTO       `json:"blocks"`
	Options      OptionsDTO       `json:"options"`
}

// AssignmentDTO is one gifter→recipient pair in a solution.
type AssignmentDTO struct {
	GifterID    string `json:"gifter_id"`
	RecipientID string `json:"recipient_id"`
}

// SolutionDTO is one ranked solution.
type SolutionDTO struct {
	Assignments []AssignmentDTO `json:"assignments"`
	Cycles      [][]string      `json:"cycles"`
	Score       ScoreDTO        `json:"score"`
}

// ScoreDTO captures solution quality metrics.
type ScoreDTO struct {
	MinCycleLen int `json:"min_cycle_len"`
	NumCycles   int `json:"num_cycles"`
	MaxCycleLen int `json:"max_cycle_len"`
}

// SolveResponse is the body of a successful POST /api/v1/solve response.
type SolveResponse struct {
	Solutions []SolutionDTO `json:"solutions"`
	Feasible  bool          `json:"feasible"`
	SeedUsed  int64         `json:"seed_used"`
}

// ErrorResponse is returned on 4xx responses.
type ErrorResponse struct {
	Feasible bool   `json:"feasible"`
	Error    string `json:"error"`
}
