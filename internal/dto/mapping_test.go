package dto_test

import (
	"testing"

	"github.com/cbochs/gift-exchange/internal/dto"
	ge "github.com/cbochs/gift-exchange/lib"
)

func TestParticipantRoundtrip(t *testing.T) {
	orig := ge.Participant{ID: "alice", Name: "Alice"}
	got := dto.ParticipantToLib(dto.ParticipantFromLib(orig))
	if got != orig {
		t.Errorf("roundtrip: got %+v, want %+v", got, orig)
	}
}

func TestParticipantsToLib(t *testing.T) {
	ds := []dto.ParticipantDTO{
		{ID: "a", Name: "Alice"},
		{ID: "b", Name: "Bob"},
	}
	ps := dto.ParticipantsToLib(ds)
	if len(ps) != 2 {
		t.Fatalf("expected 2, got %d", len(ps))
	}
	if ps[0].ID != "a" || ps[0].Name != "Alice" {
		t.Errorf("unexpected ps[0]: %+v", ps[0])
	}
	if ps[1].ID != "b" || ps[1].Name != "Bob" {
		t.Errorf("unexpected ps[1]: %+v", ps[1])
	}
}

func TestBlockRoundtrip(t *testing.T) {
	orig := ge.Block{From: "a", To: "b"}
	got := dto.BlockToLib(dto.BlockFromLib(orig))
	if got != orig {
		t.Errorf("roundtrip: got %+v, want %+v", got, orig)
	}
}

func TestBlocksToLib(t *testing.T) {
	ds := []dto.BlockDTO{{From: "a", To: "b"}, {From: "c", To: "d"}}
	bs := dto.BlocksToLib(ds)
	if len(bs) != 2 {
		t.Fatalf("expected 2, got %d", len(bs))
	}
	if bs[0].From != "a" || bs[0].To != "b" {
		t.Errorf("unexpected bs[0]: %+v", bs[0])
	}
}

func TestSolutionFromLib(t *testing.T) {
	s := ge.Solution{
		Assignments: []ge.Assignment{
			{GifterID: "a", RecipientID: "b"},
			{GifterID: "b", RecipientID: "a"},
		},
		Cycles: []ge.Cycle{{"a", "b"}},
		Score:  ge.Score{MinCycleLen: 2, NumCycles: 1, MaxCycleLen: 2},
	}
	d := dto.SolutionFromLib(s)

	if len(d.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(d.Assignments))
	}
	if d.Assignments[0].GifterID != "a" || d.Assignments[0].RecipientID != "b" {
		t.Errorf("unexpected assignment[0]: %+v", d.Assignments[0])
	}
	if len(d.Cycles) != 1 || len(d.Cycles[0]) != 2 {
		t.Errorf("unexpected cycles: %v", d.Cycles)
	}
	if d.Score.MinCycleLen != 2 || d.Score.NumCycles != 1 || d.Score.MaxCycleLen != 2 {
		t.Errorf("unexpected score: %+v", d.Score)
	}
}

func TestBuildProblem_Basic(t *testing.T) {
	participants := []dto.ParticipantDTO{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}}
	blocks := []dto.BlockDTO{{From: "a", To: "b"}}
	prob := dto.BuildProblem(participants, blocks, nil, nil)
	if len(prob.Participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(prob.Participants))
	}
	if len(prob.Blocks) != 1 || prob.Blocks[0].From != "a" || prob.Blocks[0].To != "b" {
		t.Errorf("unexpected blocks: %+v", prob.Blocks)
	}
}

func TestBuildProblem_RelationshipExpands(t *testing.T) {
	participants := []dto.ParticipantDTO{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}}
	rels := []dto.RelationshipDTO{{A: "a", B: "b"}}
	prob := dto.BuildProblem(participants, nil, rels, nil)
	if len(prob.Blocks) != 2 {
		t.Fatalf("expected 2 blocks from relationship expansion, got %d", len(prob.Blocks))
	}
	hasAB := prob.Blocks[0].From == "a" && prob.Blocks[0].To == "b" ||
		prob.Blocks[1].From == "a" && prob.Blocks[1].To == "b"
	hasBA := prob.Blocks[0].From == "b" && prob.Blocks[0].To == "a" ||
		prob.Blocks[1].From == "b" && prob.Blocks[1].To == "a"
	if !hasAB || !hasBA {
		t.Errorf("relationship did not expand to both directions: %+v", prob.Blocks)
	}
}

func TestBuildProblem_DisabledParticipant(t *testing.T) {
	participants := []dto.ParticipantDTO{
		{ID: "a", Name: "Alice"},
		{ID: "b", Name: "Bob", Disabled: true},
		{ID: "c", Name: "Carol"},
	}
	blocks := []dto.BlockDTO{
		{From: "a", To: "b"}, // involves disabled participant
		{From: "a", To: "c"}, // should remain
	}
	rels := []dto.RelationshipDTO{
		{A: "b", B: "c"}, // involves disabled participant
	}
	prob := dto.BuildProblem(participants, blocks, rels, nil)
	if len(prob.Participants) != 2 {
		t.Fatalf("expected 2 active participants, got %d", len(prob.Participants))
	}
	if len(prob.Blocks) != 1 || prob.Blocks[0].From != "a" || prob.Blocks[0].To != "c" {
		t.Errorf("expected only a→c block, got %+v", prob.Blocks)
	}
}

func TestBuildProblem_DisabledBlock(t *testing.T) {
	participants := []dto.ParticipantDTO{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}}
	blocks := []dto.BlockDTO{
		{From: "a", To: "b", Disabled: true},
	}
	prob := dto.BuildProblem(participants, blocks, nil, nil)
	if len(prob.Blocks) != 0 {
		t.Errorf("expected no blocks, got %+v", prob.Blocks)
	}
}

func TestBuildProblem_DisabledRelationship(t *testing.T) {
	participants := []dto.ParticipantDTO{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}}
	rels := []dto.RelationshipDTO{{A: "a", B: "b", Disabled: true}}
	prob := dto.BuildProblem(participants, nil, rels, nil)
	if len(prob.Blocks) != 0 {
		t.Errorf("expected no blocks from disabled relationship, got %+v", prob.Blocks)
	}
}

func TestBuildProblem_DisabledGroup(t *testing.T) {
	participants := []dto.ParticipantDTO{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}, {ID: "c", Name: "Carol"}}
	blocks := []dto.BlockDTO{
		{From: "a", To: "b", Group: "g1"},           // in disabled group
		{From: "a", To: "c"},                          // no group, should remain
		{From: "b", To: "c", Group: "g2", Disabled: true}, // different group, also disabled directly
	}
	groups := []dto.BlockGroupDTO{{ID: "g1", Label: "History 2024", Disabled: true}}
	prob := dto.BuildProblem(participants, blocks, nil, groups)
	if len(prob.Blocks) != 1 || prob.Blocks[0].From != "a" || prob.Blocks[0].To != "c" {
		t.Errorf("expected only a→c block, got %+v", prob.Blocks)
	}
}

func TestBuildProblem_EmptyGroupIDNotFiltered(t *testing.T) {
	// A block with no group (Group == "") must not be filtered even if a disabled
	// group exists, since disabledGroups[""] is never set.
	participants := []dto.ParticipantDTO{{ID: "a", Name: "Alice"}, {ID: "b", Name: "Bob"}}
	blocks := []dto.BlockDTO{{From: "a", To: "b"}}
	groups := []dto.BlockGroupDTO{{ID: "g1", Label: "Some Group", Disabled: true}}
	prob := dto.BuildProblem(participants, blocks, nil, groups)
	if len(prob.Blocks) != 1 {
		t.Errorf("expected block with no group to survive disabled group: got %+v", prob.Blocks)
	}
}

func TestSolutionsFromLib(t *testing.T) {
	ss := []ge.Solution{
		{Score: ge.Score{MinCycleLen: 4, NumCycles: 1, MaxCycleLen: 4}},
		{Score: ge.Score{MinCycleLen: 2, NumCycles: 2, MaxCycleLen: 2}},
	}
	ds := dto.SolutionsFromLib(ss)
	if len(ds) != 2 {
		t.Fatalf("expected 2, got %d", len(ds))
	}
	if ds[0].Score.MinCycleLen != 4 || ds[1].Score.NumCycles != 2 {
		t.Errorf("unexpected scores: %+v, %+v", ds[0].Score, ds[1].Score)
	}
}
