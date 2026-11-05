# Gift Exchange

[![CI](https://github.com/cbochs/gift-exchange/actions/workflows/ci.yml/badge.svg)](https://github.com/cbochs/gift-exchange/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/cbochs/gift-exchange)](https://github.com/cbochs/gift-exchange/releases)
[![Written in Go](https://img.shields.io/badge/Go-1.27.3-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![GHCR](https://img.shields.io/badge/ghcr.io-cbochs%2Fgift--exchange-blue?logo=docker&logoColor=white)](https://github.com/cbochs/gift-exchange/pkgs/container/gift-exchange)

A small tool and simple web UI for creating the perfect[^1] gift exchange.

## Motivation

You want to put together a secret gift exchange with a small group of
friends. You also want to make sure it's random. The easiest option is the ol'
paper-slips-in-a-bag, you write everyone's name down on a slip of paper, place
them in a bag, and everyone takes a slip. What if they choose their own name?
Choose again. What if there are people in the group (i.e. partners) who already
gift each other? Choose another. What if this becomes an annual tradition? You
may not want friends gifting the same people. Choose another slip.

People finally has their slip, time passes, and everyone sits down for the gift
exchange. You pick someone to start the festivities and they announce who they
brought for. The recipient opens their gift with excitement, then it moves on
to their turn to announce who their gift is for. This goes on, but before
everyone has received their gift it comes back to the person who started the
exchange. The excitement fades a bit, but you pick the next person in the
circle of friends and continue. This loop-closing may happen once or more. You
may even have friends gift each other. In the end, everyone has received their
gift and is thanking the person who gifted it to them. The gift exchange was
successful! But there's a bit of room for improvement, at least with respect to
how we put the pairings together.

That is why this project exists. It creates gift exchanges where **no one
receives themselves**, **no one is paired with their partner**, **no one is
paired with the same person they had in previous years**, **all pairings
attempt to form a complete loop**, and a bunch of other goodies (multiple
solutions, reproducible randomness, graphical visualizations, full
customization, and more).

## Install

### Web app (Docker)

Run a released image from GitHub Container Registry, replacing `<version>` with
a release tag (for example, `v1.2.3`):

```bash
docker run --rm -p 8080:8080 ghcr.io/cbochs/gift-exchange:<version>
```

Then open [http://localhost:8080](http://localhost:8080). Images are published
for `linux/amd64` and `linux/arm64`. See the
[package page](https://github.com/cbochs/gift-exchange/pkgs/container/gift-exchange)
for available tags.

### CLI

Install the command-line tool with Go:

```bash
go install github.com/cbochs/gift-exchange/cmd/giftexchange@latest
```

Prebuilt CLI binaries for Linux (amd64, arm64) and macOS (arm64) are also
available on the [GitHub Releases page](https://github.com/cbochs/gift-exchange/releases).

## How It Works

Each participant is a node in a directed graph. An edge from A to B means A may
give a gift to B. The solver removes self-assignments and any blocked pairings;
two-way relationships such as partners or siblings are treated as blocks in
both directions. It then searches for an assignment in which every participant
gives to exactly one person and receives from exactly one person.

Assignments form one or more cycles. The solver first searches for a single
cycle containing everyone. If that is impossible, it progressively lowers the
minimum cycle-length requirement until it finds feasible cycle covers. It can
return multiple distinct solutions (five by default), ranked to favor longer
minimum cycles, fewer cycles, and then a longer largest cycle. Reusing a seed
makes a run reproducible.

The web UI lets you configure participants, relationship constraints, and
directed blocks (including prior-year pairings), then inspect the ranked results
as a graph and as cycle lists. The same solver is available as a Go library, an
HTTP API, and a CLI for scripting.

## Components

| Component | Location | Description |
| -- | -- | -- |
| **Library** | `lib/` | Core solver — pure Go, no dependencies |
| **CLI** | `cmd/giftexchange/` | Command-line tool wrapping the library |
| **Web backend** | `server/` | HTTP API server wrapping the library |
| **Web frontend** | `server/web/` | Browser UI consuming the HTTP API |

______________________________________________________________________

## Web App

Start the server and open `http://localhost:8080`:

```bash
go run ./cmd/server/ [flags]
```

Flags:

- `--addr <addr>` — listen address (default `:8080`)
- `--cors-origin <origin>` — allowed CORS origin (default `*`)
- `--timeout <duration>` — request timeout (default `15s`)
- `--static <dir>` — serve a static frontend from this directory at `/`

> [!NOTE]
> Pass `--static server/web/` during development to serve live files without rebuilding.

The UI has two panels. The **left panel** is the form: add participants,
configure blocks (directed, e.g. history) and relationships (symmetric, e.g.
partners/siblings), and click **Generate**. The **right panel** shows results:
a force-directed graph of valid pairings with the selected solution highlighted
in color, and ranked solution tabs listing each assignment grouped by cycle.

Use **Add as history blocks** to carry this year's assignments forward as next
year's blocks. Use **Download / Import JSON** to save and restore the full
problem state — cached solutions are displayed immediately on import with no
API call.

![Web UI](https://github.com/user-attachments/assets/2e3d674b-47f7-4b69-824e-2be41fdf852e)

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

______________________________________________________________________

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

[^1]: <https://en.wikipedia.org/wiki/Hamiltonian_path>
