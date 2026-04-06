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
