// Package dto provides wire types shared between the HTTP server and CLI,
// and mapping functions to convert to and from the lib domain types.
package dto

// ParticipantDTO is a participant in a gift exchange problem.
type ParticipantDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Disabled bool   `json:"disabled,omitempty"`
}

// BlockDTO forbids a specific directed pairing.
type BlockDTO struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Disabled bool   `json:"disabled,omitempty"`
	Group    string `json:"group,omitempty"`
}

// RelationshipDTO forbids a bidirectional pairing. It expands to two directed
// blocks (A→B and B→A) when building the solver problem.
type RelationshipDTO struct {
	A        string `json:"a"`
	B        string `json:"b"`
	Disabled bool   `json:"disabled,omitempty"`
}

// BlockGroupDTO carries the server-relevant state for a named group of blocks.
// The collapsed field (frontend-only UI state) is intentionally absent.
type BlockGroupDTO struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled,omitempty"`
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
