# Phase 4 — Web Backend

## Status

- [x] `server/api.go` — all DTO types (`SolveRequest`, `SolveResponse`, `OptionsDTO`, etc.)
- [x] `server/handlers.go` — `solveHandler`, `healthHandler`, CORS middleware, `dtoToProblem`
- [x] `server/main.go` — flag parsing, `newServer`, `http.ListenAndServe`
- [x] `server/handlers_test.go` — all handler tests passing via `httptest`
- [x] Manual curl smoke test against running server
- [ ] Single-binary deployment verified (`--static web/` serves frontend)

## Goal

Wrap the `lib/` package in a stateless HTTP API using `net/http`. The backend has no database, no sessions, and no persistent state. Every request is self-contained. This phase also finalizes the API contract that the frontend will consume.

---

## 1. High-Level Design

### 1.1 Endpoints

```
POST /api/v1/solve     — Submit a problem, receive ranked solutions
GET  /api/v1/health    — Liveness probe
```

No authentication. The server is intended for local or trusted-network use. CORS headers are configured to allow the frontend origin.

### 1.2 API Contract

**`POST /api/v1/solve`**

Request body (JSON):

```json
{
  "participants": [
    { "id": "alice", "name": "Alice Smith" },
    { "id": "bob", "name": "Bob Jones" },
    { "id": "carol", "name": "Carol Wu" },
    { "id": "dave", "name": "Dave Kim" }
  ],
  "blocks": [{ "from": "alice", "to": "bob" }],
  "options": {
    "max_solutions": 5,
    "seed": 42,
    "timeout_ms": 5000
  }
}
```

All `options` fields are optional with defaults: `max_solutions=5`, `seed=0` (random), `timeout_ms=10000`. There is no `min_cycle_len` — the solver automatically finds the best achievable minimum cycle length via the N/M progression.

Success response `200 OK`:

```json
{
  "solutions": [
    {
      "assignments": [
        { "gifter_id": "alice", "recipient_id": "carol" },
        { "gifter_id": "carol", "recipient_id": "dave" },
        { "gifter_id": "dave", "recipient_id": "bob" },
        { "gifter_id": "bob", "recipient_id": "alice" }
      ],
      "cycles": [["alice", "carol", "dave", "bob"]],
      "score": {
        "min_cycle_len": 4,
        "num_cycles": 1,
        "max_cycle_len": 4
      }
    }
  ],
  "feasible": true,
  "seed_used": 42
}
```

Error response (validation failure) `400 Bad Request`:

```json
{
  "feasible": false,
  "error": "block references unknown participant: \"xavier\""
}
```

Error response (infeasible problem) `422 Unprocessable Entity`:

```json
{
  "feasible": false,
  "error": "no valid assignment exists under the given constraints"
}
```

**`GET /api/v1/health`**

Response `200 OK`:

```json
{ "status": "ok" }
```

### 1.3 Request Types (`server/api.go`)

`server/api.go` is the canonical definition of the API contract. It contains only types — no business logic, no HTTP imports. This file is the shared language between frontend and backend.

```go
// ParticipantDTO is a participant in a gift exchange problem.
type ParticipantDTO struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

// BlockDTO forbids a specific directed pairing.
type BlockDTO struct {
    From string `json:"from"`
    To   string `json:"to"`
}

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
    Participants []ParticipantDTO `json:"participants"`
    Blocks       []BlockDTO       `json:"blocks"`
    Options      OptionsDTO       `json:"options"`
}

// AssignmentDTO is one gifter→recipient pair in a solution.
type AssignmentDTO struct {
    GifterID    string `json:"gifter_id"`
    RecipientID string `json:"recipient_id"`
}

// SolutionDTO is one ranked solution.
type SolutionDTO struct {
    Assignments []AssignmentDTO `json:"assignments"`
    Cycles      [][]string      `json:"cycles"`
    Score       ScoreDTO        `json:"score"`
}

// ScoreDTO captures solution quality metrics.
type ScoreDTO struct {
    MinCycleLen int `json:"min_cycle_len"`
    NumCycles   int `json:"num_cycles"`
    MaxCycleLen int `json:"max_cycle_len"`
}

// SolveResponse is the body of a successful POST /api/v1/solve response.
type SolveResponse struct {
    Solutions []SolutionDTO `json:"solutions"`
    Feasible  bool          `json:"feasible"`
    SeedUsed  int64         `json:"seed_used"`
}

// ErrorResponse is returned on 4xx responses.
type ErrorResponse struct {
    Feasible bool   `json:"feasible"`
    Error    string `json:"error"`
}
```

### 1.4 Handler Design

`server/handlers.go` contains one handler per endpoint. Handlers follow a consistent pattern:

1. Decode request body
2. Map DTO → library types
3. Call library
4. Map library types → response DTO
5. Encode response

Error handling is centralized through a `writeError(w, status, msg)` helper. No error is swallowed — all errors reach the response.

### 1.5 Middleware

A minimal middleware chain is applied to all routes:

1. **CORS**: `Access-Control-Allow-Origin: *` (configurable via `--cors-origin` flag) + preflight `OPTIONS` handling
2. **Content-Type enforcement**: POST handlers return `415` if `Content-Type` is not `application/json`
3. **Request size limit**: `http.MaxBytesReader(w, r.Body, 1<<20)` (1 MB) to prevent runaway inputs
4. **Timeout**: `http.TimeoutHandler` wraps each handler with a server-side deadline (default 15s, configurable)

### 1.6 Server Configuration

```
giftexchange-server [flags]
  --addr        string   listen address (default ":8080")
  --cors-origin string   allowed origin for CORS (default "*")
  --timeout     duration request timeout (default 15s)
  --static      string   directory to serve static frontend files (optional)
```

When `--static` is provided, the server also serves the frontend at `/`. This enables a single-binary deployment.

### 1.7 Testing Strategy

Tests use `httptest.NewRecorder()` and `httptest.NewServer()` — no real network binding.

**Unit tests (`server/handlers_test.go`):**

- `TestSolveHandler_OK`: valid 4-participant request → 200 with ≥1 solution
- `TestSolveHandler_InvalidJSON`: malformed body → 400
- `TestSolveHandler_UnknownParticipant`: block with unknown ID → 400
- `TestSolveHandler_Infeasible`: all pairs blocked → 422
- `TestSolveHandler_ReproducibleSeed`: same seed → same solution in response
- `TestSolveHandler_ContentType`: missing Content-Type → 415
- `TestSolveHandler_BodyTooLarge`: body > 1MB → 413
- `TestHealthHandler`: GET /health → 200 `{"status":"ok"}`

**Integration test (round-trip with library):**

- `TestSolveHandler_PropertyValid`: for random valid inputs, every returned solution passes the property validator from Phase 2

---

## 2. Implementation Plan

1. **Create `server/api.go`**: all DTO types — no dependencies on `lib/`
2. **Create `server/handlers.go`**: `solveHandler`, `healthHandler`, mapping functions
3. **Create `server/main.go`**: `newServer(opts) *http.ServeMux`, CLI flag parsing, `http.ListenAndServe`
4. **Implement CORS and middleware** as wrapping functions
5. **Write `server/handlers_test.go`**: all handler tests using `httptest`
6. **Manually test** using `curl` against a running server and the existing `participants.json` as input
7. **Address any library API friction** (coordinate with Phase 2 library changes)

---

## 3. Implementation Snippets

### `server/handlers.go` — Solve handler

```go
func (h *handler) solveHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

    var req SolveRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
        return
    }

    prob, opts, seed := dtoToProblem(req)

    solutions, err := giftexchange.Solve(r.Context(), prob, opts)
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
```

### `server/handlers.go` — CORS middleware

```go
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
```

### `server/handlers.go` — DTO mapping

```go
func dtoToProblem(req SolveRequest) (ge.Problem, ge.Options, int64) {
    participants := make([]ge.Participant, len(req.Participants))
    for i, p := range req.Participants {
        participants[i] = ge.Participant{ID: p.ID, Name: p.Name}
    }
    blocks := make([]ge.Block, len(req.Blocks))
    for i, b := range req.Blocks {
        blocks[i] = ge.Block{From: b.From, To: b.To}
    }

    opts := req.Options
    seed := opts.Seed
    if seed == 0 { seed = time.Now().UnixNano() }

    timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
    if timeout == 0 { timeout = 10 * time.Second }

    // Problem has no MinCycleLen; the library manages the N/M progression internally.
    return ge.Problem{
            Participants: participants,
            Blocks:       blocks,
        }, ge.Options{
            MaxSolutions: max(opts.MaxSolutions, 1),
            Seed:         seed,
            Timeout:      timeout,
        }, seed
}
```

### `server/handlers_test.go` — Handler test helper

```go
func testSolve(t *testing.T, body string) (resp *http.Response, respBody []byte) {
    t.Helper()
    h := newHandler()
    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/api/v1/solve",
        strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    h.solveHandler(rec, req)
    return rec.Result(), rec.Body.Bytes()
}

func TestSolveHandler_OK(t *testing.T) {
    body := `{"participants":[
        {"id":"a","name":"Alice"},{"id":"b","name":"Bob"},
        {"id":"c","name":"Carol"},{"id":"d","name":"Dave"}
    ],"blocks":[],"options":{"seed":1}}`

    resp, raw := testSolve(t, body)
    require.Equal(t, 200, resp.StatusCode)

    var result SolveResponse
    require.NoError(t, json.Unmarshal(raw, &result))
    require.True(t, result.Feasible)
    require.NotEmpty(t, result.Solutions)
}
```
