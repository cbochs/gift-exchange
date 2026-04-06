# Phase 10 — State Persistence & Link Sharing

## Status — COMPLETE

- [x] **P1** — LocalStorage save/load (input state only)
- [x] **P2** — Reset button (clears state + localStorage)
- [x] **P3** — Compact URL encoding helpers (`encodeStateToHash` / `decodeStateFromHash`)
- [x] **P4** — On-load hash detection and application
- [x] **P5** — "Copy Link" button + shared-link banner

**Post-implementation fixes:**
- Hash cleared from URL immediately after apply (prevents stale hash re-applying on reload)
- Banner suppressed when decoded hash has no participants (empty-state copy-link edge case)
- Banner CSS changed to `.hash-banner:not([hidden])` — `display:flex` was overriding the `hidden` attribute, making the banner always visible and the dismiss button non-functional

---

## Goal

Two features, independently deliverable:

1. **LocalStorage persistence** — form inputs survive a page refresh without the user having to re-import a JSON file.
2. **Link sharing** — a "Copy Link" button encodes the current problem into a URL fragment that pre-populates the form when opened, with no backend state needed.

Both features intentionally avoid auto-solving on load. The user always clicks **Generate** explicitly.

---

## P1 — LocalStorage Save/Load

### What to persist

Only the problem _inputs_: `participants`, `relationships`, `blocks`, `options`. Not solutions — they are fast to recompute and excluding them avoids stale result display on refresh.

### Storage key

`gift-exchange-v1` (version suffix allows a clean schema migration by bumping to `v2` and ignoring data under old keys, rather than crashing on an unexpected shape).

### When to save

After every state mutation that touches inputs. The natural hook is a `saveState()` call at the tail of `renderSidebar()`, which is already invoked after every input change. Debounce to ~300 ms to avoid thrashing during rapid edits.

```js
function saveState() {
  const payload = {
    participants: state.participants,
    relationships: state.relationships,
    blocks: state.blocks,
    options: state.options,
  };
  localStorage.setItem("gift-exchange-v1", JSON.stringify(payload));
}
```

### On load

`loadState()` at startup — deserializes from localStorage and calls the existing `applyImport()` path with `needsSolve = false` (populate form, do not auto-solve).

If the stored JSON is malformed or missing required fields, fail silently and start from an empty state (treat it as a cold start).

### Interaction with link sharing (P4)

If a URL hash is present on load, it takes priority over localStorage. Apply the hash state first; only begin writing to localStorage once the user makes their first edit. This prevents clobbering a user's personal saved problem with someone else's shared link.

---

## P2 — Reset Button

A "Reset" or "Clear all" action that:

1. Resets `state` to the empty initial values
2. Calls `localStorage.removeItem("gift-exchange-v1")`
3. Clears `location.hash` (so the URL doesn't re-populate on refresh)
4. Re-renders all panels

This is a necessary escape hatch for localStorage (no other way to clear a stale saved state) and improves general UX. Place it in the existing toolbar alongside **Download** and **Import**.

---

## P3 — Compact URL Encoding

### Why a separate encoding

The download JSON format is intentionally human-readable and round-trippable (long keys, expanded structure). URL fragments need to be as short as possible for practical shareability.

### Size analysis

Blocks store participant IDs as string slugs (e.g. `"alice"`, `"bob-smith"`), not integers. A block serialized as a JSON object is ~30–38 chars; as an index pair it is ~5–7 chars. For a realistic large group (15 participants, 105 blocks — the cousins_2026 data):

| Encoding                     | JSON chars | Base64 chars |
| ---------------------------- | ---------- | ------------ |
| Full JSON objects (slugs)    | ~4,500     | ~6,000       |
| Compact format (int indices) | ~900       | ~1,200       |

The compact format is comfortably shareable via messaging apps; the full format may be truncated in link previews.

### Compact format schema (v1)

```json
{
  "v": 1,
  "p": [
    ["alice", "Alice"],
    ["bob", "Bob"]
  ],
  "b": [
    [0, 1],
    [1, 2]
  ],
  "r": [[0, 1]],
  "o": { "m": 5, "s": 42 }
}
```

| Key | Meaning                                                |
| --- | ------------------------------------------------------ |
| `v` | Format version (integer)                               |
| `p` | Participants as `[id, name]` tuples                    |
| `b` | Blocks as `[from_index, to_index]` index pairs         |
| `r` | Relationships as `[a_index, b_index]` index pairs      |
| `o` | Options: `m` = maxSolutions, `s` = seed (omit if null) |

Index pairs reference positions in `p`. Options fields are omitted when equal to defaults.

### URL format

```
https://example.com/#v1:<base64url>
```

Use URL-safe base64 (`+` → `-`, `/` → `_`, no padding `=`) so the fragment needs no percent-encoding.

### Helpers

```js
function encodeStateToHash(state) {
  const compact = { v: 1, p: [...], b: [...], r: [...], o: {...} };
  const json = JSON.stringify(compact);
  const b64 = btoa(json).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
  return '#v1:' + b64;
}

function decodeStateFromHash(hash) {
  // Returns a partial state object or null on any parse failure
}
```

`decodeStateFromHash` must be defensive: return `null` on any failure (malformed base64, unknown version, missing required fields) so the caller can fall back to localStorage or an empty state.

---

## P4 — On-Load Hash Detection

On `DOMContentLoaded`, before checking localStorage:

```
if location.hash starts with "#v1:"
  → decode hash
  → if valid: apply to state (don't auto-solve, don't save to localStorage yet)
  → if invalid: ignore silently, fall through to localStorage
else
  → check localStorage
```

After the user makes their first edit following a hash load, begin persisting to localStorage normally. This avoids overwriting the user's saved problem with someone else's shared link during read-only exploration.

A simple `hashApplied` flag tracks whether the initial state came from a hash, so `saveState()` can skip writes until the first mutation.

---

## P5 — "Copy Link" Button + Shared-Link Banner

### Copy Link button

Located in the toolbar alongside Download and Import. On click:

1. Call `encodeStateToHash(state)` to produce `#v1:<base64url>`
2. Construct the full URL: `window.location.origin + window.location.pathname + hash`
3. Call `navigator.clipboard.writeText(url)`
4. Show brief feedback on the button ("Copied!" for ~2s, then revert label)

> [!NOTE]
> `navigator.clipboard` requires a secure context (HTTPS or localhost). The app is served by the Go server, so this is satisfied in all expected deployments.

### Shared-link banner

When the form is pre-populated from a URL hash on load, show a dismissible banner at the top of the sidebar:

> Loaded from a shared link — click **Generate** to solve, or edit the form.

Dismiss on click or after 8s. This prevents user confusion about unexpected form content.

---

## Implementation Order

Work through P1–P5 in order. Each is independently committable.

| Item | Files touched                             | Dependencies |
| ---- | ----------------------------------------- | ------------ |
| P1   | `app.js` (saveState, loadState, debounce) | none         |
| P2   | `app.js`, `index.html` (Reset button)     | none         |
| P3   | `app.js` (encode/decode helpers + tests)  | none         |
| P4   | `app.js` (startup logic)                  | P1, P3       |
| P5   | `app.js`, `index.html`, `style.css`       | P3, P4       |

---

## Explicitly Out of Scope

- **Compression** (`CompressionStream` / gzip): async complexity is not worth it for current group sizes. Revisit if real-world URLs exceed ~4,000 chars.
- **Server-side short links**: stateless design is a core project principle.
- **Auto-solve on load**: always wait for the user to click Generate.
- **Multi-tab sync** (`storage` event): not needed for single-tab usage.
