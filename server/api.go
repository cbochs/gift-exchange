package server

import "github.com/cbochs/gift-exchange/internal/dto"

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
	Participants  []dto.ParticipantDTO  `json:"participants"`
	Blocks        []dto.BlockDTO        `json:"blocks,omitempty"`
	Relationships []dto.RelationshipDTO `json:"relationships,omitempty"`
	BlockGroups   []dto.BlockGroupDTO   `json:"block_groups,omitempty"`
	Options       OptionsDTO            `json:"options,omitempty"`
}

// SolveResponse is the body of a successful POST /api/v1/solve response.
type SolveResponse struct {
	Solutions []dto.SolutionDTO `json:"solutions"`
	Feasible  bool              `json:"feasible"`
	SeedUsed  int64             `json:"seed_used"`
}

// ErrorResponse is returned on error responses.
type ErrorResponse struct {
	Feasible bool   `json:"feasible"`
	Error    string `json:"error"`
}
