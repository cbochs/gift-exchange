# Gift Exchange

A tool for creating optimized gift exchange assignments. Given a list of participants and optional blocked pairings (e.g. spouses, prior-year pairs), it finds valid assignments where everyone gives and receives exactly one gift — maximizing cycle length so participants are part of larger, more interesting exchange loops rather than simple back-and-forth pairs.

Multiple ranked solutions are returned so organizers can choose.

## Components

| Component        | Location            | Description                            |
| ---------------- | ------------------- | -------------------------------------- |
| **Library**      | `lib/`              | Core solver — pure Go, no dependencies |
| **CLI**          | `cmd/giftexchange/` | Command-line tool wrapping the library |
| **Web backend**  | `server/`           | HTTP API server wrapping the library   |
| **Web frontend** | `server/web/`       | Browser UI consuming the HTTP API      |

---

## Web UI

Start the server and open `http://localhost:8080`:

```bash
go run ./cmd/server/
```

> [!NOTE]
> Pass `--static server/web/` during development to serve live files without rebuilding.

The UI has two panels. The **left panel** is the form: add participants, configure blocks (directed, e.g. history) and relationships (symmetric, e.g. partners/siblings), and click **Generate**. The **right panel** shows results: a force-directed graph of valid pairings with the selected solution highlighted in color, and ranked solution tabs listing each assignment grouped by cycle.

Use **Add as history blocks** to carry this year's assignments forward as next year's blocks. Use **Download / Import JSON** to save and restore the full problem state — cached solutions are displayed immediately on import with no API call.

---

## Web Backend

### Running the server

```bash
go run ./cmd/server/ [flags]
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

All `options` fields are optional. Defaults: `max_solutions=5`, `seed=random`, `timeout_ms=15000`.

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

---

## CLI

A thin wrapper around the library for scripting and offline use.

```bash
go install github.com/cbochs/gift-exchange/cmd/giftexchange@latest
```

Pass a problem via stdin and get assignments back as JSON:

```bash
echo '{
  "participants": [
    {"id":"alice","name":"Alice"},
    {"id":"bob","name":"Bob"},
    {"id":"carol","name":"Carol"},
    {"id":"dave","name":"Dave"}
  ],
  "blocks": [{"from":"alice","to":"bob"}]
}' | giftexchange solve -input - -json
```

Subcommands: `solve`, `validate`, `analyze`. Run `giftexchange <subcommand> -help` for flags.
