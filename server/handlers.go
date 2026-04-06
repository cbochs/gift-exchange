package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cbochs/gift-exchange/internal/dto"
	ge "github.com/cbochs/gift-exchange/lib"
)

func solveHandler(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req SolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	prob, opts, seed := dtoToProblem(req)

	solutions, err := ge.Solve(r.Context(), prob, opts)
	if err != nil {
		switch {
		case errors.Is(err, ge.ErrInvalid):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ge.ErrInfeasible):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, SolveResponse{
		Solutions: dto.SolutionsFromLib(solutions),
		Feasible:  true,
		SeedUsed:  seed,
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
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
	seed := req.Options.Seed
	if seed == 0 {
		seed = ge.NewSeed()
	}

	maxSolutions := req.Options.MaxSolutions
	if maxSolutions <= 0 {
		maxSolutions = ge.DefaultMaxSolutions
	}

	timeout := time.Duration(req.Options.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return ge.Problem{
		Participants: dto.ParticipantsToLib(req.Participants),
		Blocks:       dto.BlocksToLib(req.Blocks),
	}, ge.Options{
		MaxSolutions: maxSolutions,
		Seed:         seed,
		Timeout:      timeout,
	}, seed
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Feasible: false, Error: msg})
}
