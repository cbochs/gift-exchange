package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	ge "github.com/cbochs/gift-exchange/lib"
)

type handler struct{}

func newHandler() *handler { return &handler{} }

func (h *handler) solveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req SolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	prob, opts, seed := dtoToProblem(req)

	solutions, err := ge.Solve(r.Context(), prob, opts)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if isValidationErr(err) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, SolveResponse{
		Solutions: solutionsToDTOs(solutions),
		Feasible:  true,
		SeedUsed:  seed,
	})
}

func (h *handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// corsMiddleware adds CORS headers and handles preflight OPTIONS requests.
func corsMiddleware(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// dtoToProblem maps the request DTO to library types and resolves option defaults.
// Returns the problem, solver options, and the concrete seed used (for echoing back).
func dtoToProblem(req SolveRequest) (ge.Problem, ge.Options, int64) {
	participants := make([]ge.Participant, len(req.Participants))
	for i, p := range req.Participants {
		participants[i] = ge.Participant{ID: p.ID, Name: p.Name}
	}

	blocks := make([]ge.Block, len(req.Blocks))
	for i, b := range req.Blocks {
		blocks[i] = ge.Block{From: b.From, To: b.To}
	}

	seed := req.Options.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	maxSolutions := req.Options.MaxSolutions
	if maxSolutions <= 0 {
		maxSolutions = 5
	}

	timeout := time.Duration(req.Options.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return ge.Problem{
		Participants: participants,
		Blocks:       blocks,
	}, ge.Options{
		MaxSolutions: maxSolutions,
		Seed:         seed,
		Timeout:      timeout,
	}, seed
}

// solutionsToDTOs converts library solutions to response DTOs.
func solutionsToDTOs(solutions []ge.Solution) []SolutionDTO {
	dtos := make([]SolutionDTO, len(solutions))
	for i, s := range solutions {
		assignments := make([]AssignmentDTO, len(s.Assignments))
		for j, a := range s.Assignments {
			assignments[j] = AssignmentDTO{GifterID: a.GifterID, RecipientID: a.RecipientID}
		}
		cycles := make([][]string, len(s.Cycles))
		for j, c := range s.Cycles {
			cycles[j] = []string(c)
		}
		dtos[i] = SolutionDTO{
			Assignments: assignments,
			Cycles:      cycles,
			Score: ScoreDTO{
				MinCycleLen: s.Score.MinCycleLen,
				NumCycles:   s.Score.NumCycles,
				MaxCycleLen: s.Score.MaxCycleLen,
			},
		}
	}
	return dtos
}

// isValidationErr reports whether err is a structural validation error (→ 400)
// rather than an infeasibility error (→ 422).
func isValidationErr(err error) bool {
	return !errors.Is(err, ge.ErrInfeasible)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Feasible: false, Error: msg})
}
