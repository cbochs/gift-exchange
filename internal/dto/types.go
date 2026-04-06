// Package dto provides wire types shared between the HTTP server and CLI,
// and mapping functions to convert to and from the lib domain types.
package dto

// ParticipantDTO is a participant in a gift exchange problem.
type ParticipantDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// BlockDTO forbids or requires a specific directed pairing.
type BlockDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// AssignmentDTO is one gifter→recipient pair in a solution.
type AssignmentDTO struct {
	GifterID    string `json:"gifter_id"`
	RecipientID string `json:"recipient_id"`
}

// ScoreDTO captures solution quality metrics.
type ScoreDTO struct {
	MinCycleLen int `json:"min_cycle_len"`
	NumCycles   int `json:"num_cycles"`
	MaxCycleLen int `json:"max_cycle_len"`
}

// SolutionDTO is one ranked solution.
type SolutionDTO struct {
	Assignments []AssignmentDTO `json:"assignments"`
	Cycles      [][]string      `json:"cycles"`
	Score       ScoreDTO        `json:"score"`
}
