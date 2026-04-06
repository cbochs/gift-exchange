package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cbochs/gift-exchange/internal/dto"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testSolve(t *testing.T, body string) (*http.Response, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/solve", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	solveHandler(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

func testSolveRaw(t *testing.T, body string, contentType string) (*http.Response, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/solve", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	solveHandler(rec, req)
	return rec.Result(), rec.Body.Bytes()
}

const fourParticipants = `{
	"participants": [
		{"id":"a","name":"Alice"},
		{"id":"b","name":"Bob"},
		{"id":"c","name":"Carol"},
		{"id":"d","name":"Dave"}
	],
	"blocks": [],
	"options": {"seed": 1}
}`

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

func TestSolveHandler_OK(t *testing.T) {
	resp, raw := testSolve(t, fourParticipants)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
	}

	var result SolveResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal error: %v\nbody: %s", err, raw)
	}
	if !result.Feasible {
		t.Fatal("expected feasible=true")
	}
	if len(result.Solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
}

func TestSolveHandler_InvalidJSON(t *testing.T) {
	resp, raw := testSolve(t, `{not valid json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, raw)
	}

	var result ErrorResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.Feasible {
		t.Fatal("expected feasible=false")
	}
}

func TestSolveHandler_UnknownParticipant(t *testing.T) {
	body := `{
		"participants": [{"id":"a","name":"Alice"},{"id":"b","name":"Bob"}],
		"blocks": [{"from":"a","to":"xavier"}],
		"options": {}
	}`
	resp, raw := testSolve(t, body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, raw)
	}
}

func TestSolveHandler_Infeasible(t *testing.T) {
	// Two participants, each blocked from giving to the other — no valid assignment.
	body := `{
		"participants": [{"id":"a","name":"Alice"},{"id":"b","name":"Bob"}],
		"blocks": [{"from":"a","to":"b"},{"from":"b","to":"a"}],
		"options": {}
	}`
	resp, raw := testSolve(t, body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.StatusCode, raw)
	}

	var result ErrorResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.Feasible {
		t.Fatal("expected feasible=false")
	}
}

func TestSolveHandler_ReproducibleSeed(t *testing.T) {
	body := `{
		"participants": [
			{"id":"a","name":"Alice"},{"id":"b","name":"Bob"},
			{"id":"c","name":"Carol"},{"id":"d","name":"Dave"}
		],
		"blocks": [],
		"options": {"seed": 99999}
	}`

	_, raw1 := testSolve(t, body)
	_, raw2 := testSolve(t, body)

	var r1, r2 SolveResponse
	if err := json.Unmarshal(raw1, &r1); err != nil {
		t.Fatalf("unmarshal r1: %v", err)
	}
	if err := json.Unmarshal(raw2, &r2); err != nil {
		t.Fatalf("unmarshal r2: %v", err)
	}

	if r1.SeedUsed != r2.SeedUsed {
		t.Fatalf("seed mismatch: %d vs %d", r1.SeedUsed, r2.SeedUsed)
	}
	if len(r1.Solutions) == 0 || len(r2.Solutions) == 0 {
		t.Fatal("expected solutions in both responses")
	}

	// First solution's assignments should be identical for the same seed.
	a1 := r1.Solutions[0].Assignments
	a2 := r2.Solutions[0].Assignments
	if len(a1) != len(a2) {
		t.Fatalf("assignment count mismatch: %d vs %d", len(a1), len(a2))
	}
	for i := range a1 {
		if a1[i] != a2[i] {
			t.Fatalf("assignment[%d] mismatch: %v vs %v", i, a1[i], a2[i])
		}
	}
}

func TestSolveHandler_ContentType(t *testing.T) {
	resp, raw := testSolveRaw(t, fourParticipants, "text/plain")
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d: %s", resp.StatusCode, raw)
	}
}

func TestSolveHandler_NoContentType(t *testing.T) {
	resp, raw := testSolveRaw(t, fourParticipants, "")
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d: %s", resp.StatusCode, raw)
	}
}

func TestSolveHandler_BodyTooLarge(t *testing.T) {
	// Build a body exceeding 1MB by padding the participants name field.
	padding := strings.Repeat("x", 1<<20+1)
	body := fmt.Sprintf(`{"participants":[{"id":"a","name":"%s"},{"id":"b","name":"Bob"}],"blocks":[],"options":{}}`, padding)
	resp, raw := testSolve(t, body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 (body too large), got %d: %s", resp.StatusCode, raw)
	}
}

func TestHealthHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	healthHandler(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", result["status"])
	}
}

// TestSolveHandler_PropertyValid verifies that for random valid problems,
// every returned solution is a valid permutation: each participant appears
// exactly once as gifter and exactly once as recipient, with no self-assignments.
func TestSolveHandler_PropertyValid(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for trial := range 20 {
		n := rng.Intn(10) + 4 // 4..13 participants
		participants := make([]dto.ParticipantDTO, n)
		ids := make([]string, n)
		for i := range n {
			id := fmt.Sprintf("p%d", i)
			participants[i] = dto.ParticipantDTO{ID: id, Name: fmt.Sprintf("Person %d", i)}
			ids[i] = id
		}

		req := SolveRequest{
			Participants: participants,
			Blocks:       nil,
			Options:      OptionsDTO{Seed: int64(trial + 1), MaxSolutions: 3},
		}
		body, _ := json.Marshal(req)

		resp, raw := testSolve(t, string(body))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("trial %d: expected 200, got %d: %s", trial, resp.StatusCode, raw)
		}

		var result SolveResponse
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("trial %d: unmarshal: %v", trial, err)
		}
		if !result.Feasible || len(result.Solutions) == 0 {
			t.Fatalf("trial %d: expected feasible solution", trial)
		}

		idSet := make(map[string]bool, n)
		for _, id := range ids {
			idSet[id] = true
		}

		for si, sol := range result.Solutions {
			if len(sol.Assignments) != n {
				t.Fatalf("trial %d sol %d: expected %d assignments, got %d", trial, si, n, len(sol.Assignments))
			}
			gifters := make(map[string]int, n)
			recipients := make(map[string]int, n)
			for _, a := range sol.Assignments {
				if !idSet[a.GifterID] {
					t.Fatalf("trial %d sol %d: unknown gifter %q", trial, si, a.GifterID)
				}
				if !idSet[a.RecipientID] {
					t.Fatalf("trial %d sol %d: unknown recipient %q", trial, si, a.RecipientID)
				}
				if a.GifterID == a.RecipientID {
					t.Fatalf("trial %d sol %d: self-assignment for %q", trial, si, a.GifterID)
				}
				gifters[a.GifterID]++
				recipients[a.RecipientID]++
			}
			for _, id := range ids {
				if gifters[id] != 1 {
					t.Fatalf("trial %d sol %d: gifter %q appears %d times", trial, si, id, gifters[id])
				}
				if recipients[id] != 1 {
					t.Fatalf("trial %d sol %d: recipient %q appears %d times", trial, si, id, recipients[id])
				}
			}
		}
	}
}
