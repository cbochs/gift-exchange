package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cbochs/gift-exchange/internal/dto"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const basicInput = `{
	"participants": [
		{"id":"a","name":"Alice"},
		{"id":"b","name":"Bob"},
		{"id":"c","name":"Carol"},
		{"id":"d","name":"Dave"}
	],
	"options": {"seed": 1, "max_solutions": 3}
}`

func writeTempInput(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "input*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func runSolveCLI(t *testing.T, input string, extraArgs ...string) (stdout, stderr string, code int) {
	t.Helper()
	f := writeTempInput(t, input)
	args := append([]string{"solve", "--input", f}, extraArgs...)
	var outBuf, errBuf strings.Builder
	code = run(args, strings.NewReader(""), &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

func runValidateCLI(t *testing.T, input string) (stdout, stderr string, code int) {
	t.Helper()
	f := writeTempInput(t, input)
	args := []string{"validate", "--input", f}
	var outBuf, errBuf strings.Builder
	code = run(args, strings.NewReader(""), &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

func runAnalyzeCLI(t *testing.T, input string) (stdout, stderr string, code int) {
	t.Helper()
	f := writeTempInput(t, input)
	args := []string{"analyze", "--input", f}
	var outBuf, errBuf strings.Builder
	code = run(args, strings.NewReader(""), &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

// ---------------------------------------------------------------------------
// Tests: solve
// ---------------------------------------------------------------------------

func TestCLI_Solve_Basic(t *testing.T) {
	out, _, code := runSolveCLI(t, basicInput)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "Solution 1") {
		t.Errorf("output missing 'Solution 1':\n%s", out)
	}
	if !strings.Contains(out, "Seed:") {
		t.Errorf("output missing 'Seed:':\n%s", out)
	}
	if !strings.Contains(out, "Cycles:") {
		t.Errorf("output missing 'Cycles:':\n%s", out)
	}
}

func TestCLI_Solve_JSON(t *testing.T) {
	out, _, code := runSolveCLI(t, basicInput, "--json")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	var resp struct {
		Solutions []dto.SolutionDTO `json:"solutions"`
		Feasible  bool          `json:"feasible"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if !resp.Feasible {
		t.Error("expected feasible=true")
	}
	if len(resp.Solutions) == 0 {
		t.Error("expected at least one solution")
	}
	// Verify the solution fields are populated.
	s := resp.Solutions[0]
	if len(s.Assignments) == 0 {
		t.Error("expected assignments in solution")
	}
	if len(s.Cycles) == 0 {
		t.Error("expected cycles in solution")
	}
	if s.Score.MinCycleLen == 0 {
		t.Error("expected non-zero MinCycleLen in score")
	}
}

func TestCLI_Solve_Infeasible(t *testing.T) {
	input := `{
		"participants": [{"id":"a","name":"Alice"}, {"id":"b","name":"Bob"}],
		"blocks": [{"from":"a","to":"b"}, {"from":"b","to":"a"}]
	}`
	_, _, code := runSolveCLI(t, input)
	if code != 2 {
		t.Errorf("expected exit code 2 (infeasible), got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Tests: validate
// ---------------------------------------------------------------------------

func TestCLI_Validate_Valid(t *testing.T) {
	out, _, code := runValidateCLI(t, basicInput)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(out, "valid") {
		t.Errorf("expected 'valid' in output:\n%s", out)
	}
	if !strings.Contains(out, "Participants: 4") {
		t.Errorf("expected participant count in output:\n%s", out)
	}
}

func TestCLI_Validate_Invalid(t *testing.T) {
	input := `{
		"participants": [{"id":"a"}, {"id":"b"}],
		"blocks": [{"from":"a","to":"z"}]
	}`
	_, errOut, code := runValidateCLI(t, input)
	if code != 1 {
		t.Errorf("expected exit 1 (invalid), got %d", code)
	}
	if !strings.Contains(errOut, "z") {
		t.Errorf("expected error mentioning unknown ID 'z':\n%s", errOut)
	}
}

// ---------------------------------------------------------------------------
// Tests: analyze
// ---------------------------------------------------------------------------

func TestCLI_Analyze(t *testing.T) {
	out, _, code := runAnalyzeCLI(t, basicInput)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n", code)
	}
	for _, want := range []string{"Participants:", "Edges:", "Hall condition:", "Dead edges:", "Recipients:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// n=4 complete graph: Hall is satisfied, no dead edges.
	if !strings.Contains(out, "Hall condition: satisfied") {
		t.Errorf("expected 'Hall condition: satisfied' for complete 4-node graph:\n%s", out)
	}
	if !strings.Contains(out, "Dead edges: none") {
		t.Errorf("expected 'Dead edges: none' for complete 4-node graph:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Tests: stdin and seed
// ---------------------------------------------------------------------------

func TestCLI_Stdin(t *testing.T) {
	var outBuf, errBuf strings.Builder
	args := []string{"solve", "--input", "-"}
	code := run(args, strings.NewReader(basicInput), &outBuf, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "Solution 1") {
		t.Errorf("output missing 'Solution 1':\n%s", outBuf.String())
	}
}

func TestCLI_SeedOverride(t *testing.T) {
	// Same seed override → identical output.
	out1, _, code1 := runSolveCLI(t, basicInput, "--seed", "999")
	out2, _, code2 := runSolveCLI(t, basicInput, "--seed", "999")
	if code1 != 0 || code2 != 0 {
		t.Fatalf("unexpected exit codes: %d, %d", code1, code2)
	}
	if out1 != out2 {
		t.Errorf("same seed should produce identical output:\nrun1: %s\nrun2: %s", out1, out2)
	}
}

// ---------------------------------------------------------------------------
// Tests: round-trip
// ---------------------------------------------------------------------------

func TestCLI_RoundTrip(t *testing.T) {
	// First run: generate JSON output with a fixed seed.
	out1, _, code := runSolveCLI(t, basicInput, "--json", "--seed", "42")
	if code != 0 {
		t.Fatalf("first run: expected exit 0, got %d", code)
	}

	// Round-trip: feed the JSON output back as input.
	// The output embeds the original seed, so results should be identical.
	out2, _, code2 := runSolveCLI(t, out1, "--json")
	if code2 != 0 {
		t.Fatalf("round-trip: expected exit 0, got %d", code2)
	}

	// Both outputs must be valid JSON with the same number of solutions.
	var r1, r2 struct {
		Solutions []dto.SolutionDTO `json:"solutions"`
	}
	if err := json.Unmarshal([]byte(out1), &r1); err != nil {
		t.Fatalf("first output invalid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(out2), &r2); err != nil {
		t.Fatalf("round-trip output invalid JSON: %v", err)
	}
	if len(r1.Solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if len(r1.Solutions) != len(r2.Solutions) {
		t.Errorf("solution counts differ: first=%d round-trip=%d", len(r1.Solutions), len(r2.Solutions))
	}
	// Assignments should match between runs (same seed → same results).
	for i := range r1.Solutions {
		a1 := r1.Solutions[i].Assignments
		a2 := r2.Solutions[i].Assignments
		if len(a1) != len(a2) {
			t.Errorf("solution %d: assignment count differs", i)
			continue
		}
		for j := range a1 {
			if a1[j].GifterID != a2[j].GifterID || a1[j].RecipientID != a2[j].RecipientID {
				t.Errorf("solution %d assignment %d differs: %v vs %v", i, j, a1[j], a2[j])
			}
		}
	}
}
