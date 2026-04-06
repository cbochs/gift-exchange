package dto

import ge "github.com/cbochs/gift-exchange/lib"

// ParticipantFromLib converts a lib Participant to a ParticipantDTO.
func ParticipantFromLib(p ge.Participant) ParticipantDTO {
	return ParticipantDTO{ID: p.ID, Name: p.Name}
}

// ParticipantToLib converts a ParticipantDTO to a lib Participant.
func ParticipantToLib(d ParticipantDTO) ge.Participant {
	return ge.Participant{ID: d.ID, Name: d.Name}
}

// ParticipantsToLib converts a slice of ParticipantDTO to lib Participants.
func ParticipantsToLib(ds []ParticipantDTO) []ge.Participant {
	out := make([]ge.Participant, len(ds))
	for i, d := range ds {
		out[i] = ParticipantToLib(d)
	}
	return out
}

// BlockFromLib converts a lib Block to a BlockDTO.
func BlockFromLib(b ge.Block) BlockDTO {
	return BlockDTO{From: b.From, To: b.To}
}

// BlockToLib converts a BlockDTO to a lib Block.
func BlockToLib(d BlockDTO) ge.Block {
	return ge.Block{From: d.From, To: d.To}
}

// BlocksToLib converts a slice of BlockDTO to lib Blocks.
func BlocksToLib(ds []BlockDTO) []ge.Block {
	out := make([]ge.Block, len(ds))
	for i, d := range ds {
		out[i] = BlockToLib(d)
	}
	return out
}

// SolutionFromLib converts a lib Solution to a SolutionDTO.
func SolutionFromLib(s ge.Solution) SolutionDTO {
	assignments := make([]AssignmentDTO, len(s.Assignments))
	for i, a := range s.Assignments {
		assignments[i] = AssignmentDTO{GifterID: a.GifterID, RecipientID: a.RecipientID}
	}
	cycles := make([][]string, len(s.Cycles))
	for i, c := range s.Cycles {
		cycles[i] = []string(c)
	}
	return SolutionDTO{
		Assignments: assignments,
		Cycles:      cycles,
		Score: ScoreDTO{
			MinCycleLen: s.Score.MinCycleLen,
			NumCycles:   s.Score.NumCycles,
			MaxCycleLen: s.Score.MaxCycleLen,
		},
	}
}

// SolutionsFromLib converts a slice of lib Solutions to SolutionDTOs.
func SolutionsFromLib(ss []ge.Solution) []SolutionDTO {
	out := make([]SolutionDTO, len(ss))
	for i, s := range ss {
		out[i] = SolutionFromLib(s)
	}
	return out
}
