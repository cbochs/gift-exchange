# Gift Exchange

A tool for creating optimized gift exchange assignments. Given a list of participants and optional blocked pairings (e.g. spouses, prior-year pairs), it finds valid assignments where everyone gives and receives exactly one gift — maximizing cycle length so participants are part of larger, more interesting exchange loops rather than simple back-and-forth pairs.

Multiple ranked solutions are returned so organizers can choose.

## Components

| Component        | Location            | Description                            |
| ---------------- | ------------------- | -------------------------------------- |
| **Library**      | `lib/`              | Core solver — pure Go, no dependencies |
| **CLI**          | `cmd/giftexchange/` | Command-line tool wrapping the library |
| **Web backend**  | `server/`           | HTTP API server wrapping the library   |
| **Web frontend** | `web/`              | Browser UI consuming the HTTP API      |

---

## Web UI

Run the server with the frontend:

```bash
go run ./server/ --static web/
```

Then open `http://localhost:8080` in a browser.

- Add participants by name; IDs are auto-generated
- Add optional blocks (directed: Alice → Bob prevents Alice giving to Bob)
- Click **Generate** to solve and display ranked solutions
- The graph shows valid pairings in grey and the selected solution in color — one color per cycle
- Click solution tabs to switch between ranked results
- **Download JSON** saves the full problem + solutions; **Import JSON** restores it (cached solutions are displayed immediately with no API call)

---

## CLI

### Installation

```bash
go install github.com/cbochs/gift-exchange/cmd/giftexchange@latest
```

Or build from source:

```bash
go build -o giftexchange ./cmd/giftexchange/
```

### Input format

All subcommands read a JSON file (or stdin with `-input -`):

```json
{
  "participants": [
    { "id": "alice", "name": "Alice" },
    { "id": "bob", "name": "Bob" },
    { "id": "carol", "name": "Carol" },
    { "id": "dave", "name": "Dave" }
  ],
  "blocks": [{ "from": "alice", "to": "bob" }]
}
```

`blocks` is optional. Each block prevents a specific directed pairing (alice cannot give to bob, but bob can still give to alice).

### Subcommands

**`solve`** — find ranked gift exchange assignments:

```
$ giftexchange solve -input problem.json

Seed: 7388176239119299000
Solutions found: 5

=== Solution 1 ===
Score: min_cycle=4  cycles=1  max_cycle=4
Cycles:
  [1] alice → carol → bob → dave → alice
...
```

Flags:

- `-input <file>` — input file, or `-` for stdin
- `-seed <n>` — fix the random seed for reproducible results
- `-n <n>` — override the maximum number of solutions to return
- `-json` — output full JSON (assignments, cycles, scores, seed used)

**`validate`** — check that a problem is well-formed and potentially solvable:

```
$ giftexchange validate -input problem.json
Input is valid.
Participants: 4
Blocks: 1
```

**`analyze`** — show graph statistics:

```
$ giftexchange analyze -input problem.json
Participants:  4
Edges:         11 of 12 possible (91.7% density)
Hamiltonian:   yes
```

---

## Web Backend

### Running the server

```bash
go run ./server/ [flags]
```

Flags:

- `--addr <addr>` — listen address (default `:8080`)
- `--cors-origin <origin>` — allowed CORS origin (default `*`)
- `--timeout <duration>` — request timeout (default `15s`)
- `--static <dir>` — serve a static frontend from this directory at `/`

### Endpoints

#### `POST /api/v1/solve`

Submit a problem, receive ranked solutions.

Request:

```json
{
  "participants": [
    { "id": "alice", "name": "Alice" },
    { "id": "bob", "name": "Bob" },
    { "id": "carol", "name": "Carol" },
    { "id": "dave", "name": "Dave" }
  ],
  "blocks": [{ "from": "alice", "to": "bob" }],
  "options": {
    "max_solutions": 5,
    "seed": 42,
    "timeout_ms": 5000
  }
}
```

All `options` fields are optional. Defaults: `max_solutions=5`, `seed=random`, `timeout_ms=10000`.

Response `200 OK`:

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
      "score": { "min_cycle_len": 4, "num_cycles": 1, "max_cycle_len": 4 }
    }
  ],
  "feasible": true,
  "seed_used": 42
}
```

Error responses:

- `400 Bad Request` — malformed JSON or unknown participant ID in a block
- `422 Unprocessable Entity` — no valid assignment exists under the given constraints

#### `GET /api/v1/health`

Liveness probe. Returns `200 OK` with `{"status":"ok"}`.

### Example curl

```bash
curl -s -X POST http://localhost:8080/api/v1/solve \
  -H "Content-Type: application/json" \
  -d '{
    "participants": [
      {"id":"alice","name":"Alice"},
      {"id":"bob","name":"Bob"},
      {"id":"carol","name":"Carol"},
      {"id":"dave","name":"Dave"}
    ],
    "blocks": [],
    "options": {"seed": 42}
  }'
```
