# skyverse-save-web — design

Status: approved by user 2026-09-19 (browse + edit scope, standalone server,
upload-based file selection, lazy-expand tree, Go+htmx approach — all
confirmed during brainstorming).

## Background

`skyverse-save-tool` (sibling project, `../skyverse-save-tool`) is a Go
library (`gvas`) plus CLI (`saveview`) for reading and editing the
Skyverse game's `.sav` binary format. `gvas` already provides everything
this project needs: `Unmarshal`/`Marshal` (byte-exact round-trip),
`Property`/`File`/`ArrayValue` tree types, `Lookup` (dotted/bracketed
property-path resolution), and scalar setters
(`SetString`/`SetBool`/`SetInt32`/`SetInt64`/`SetFloat32`/`SetFloat64`).

Goal: a standalone web server, in its own repo, that imports `gvas` as a
library and gives the same browse/edit capability as the `saveview` CLI
through a browser UI — upload a `.sav`, browse its property tree, edit
scalar values, download the edited file.

This project only *consumes* `gvas`'s exported API. It never modifies
`skyverse-save-tool`.

## Module layout

```
skyverse-save-web/                 (module: skyverseweb)
  go.mod                             requires skyversesave (gvas), with a
                                      local `replace skyversesave =>
                                      ../skyverse-save-tool` directive since
                                      skyverse-save-tool has no published
                                      module path yet
  main.go                            entrypoint: parse -addr flag, wire
                                      handlers, http.ListenAndServe
  session.go                         in-memory session store
  treeview.go                        path resolution + children-listing
                                      helper built only on gvas's exported
                                      API (Property/ArrayValue fields,
                                      gvas.Lookup)
  handlers.go                        HTTP handlers (upload/children/edit/download)
  templates/
    index.html.tmpl                  page shell: upload form + empty tree container
    children.html.tmpl                renders one node's children as an
                                       <ul> fragment (used for the root
                                       after upload, and for every
                                       lazy-expand response)
    leaf_edit.html.tmpl                renders one scalar leaf's
                                        display+edit-form fragment (used
                                        after a successful/failed edit)
  static/
    htmx.min.js                       vendored locally (go:embed), not
                                       loaded from a CDN — server works
                                       fully offline
    style.css
  testdata/
    Player_Local.sav
    Player_Remote_021fa7902e50eeeb8c6ebdbe4fc922fe.sav
    WorldInfo.sav                     copied from skyverse-save-tool's
                                       testdata/, used by this project's
                                       own handler tests
  go.mod / go.sum
```

Templates and static assets are embedded into the binary via `go:embed`
so `skyverse-save-web` is a single deployable binary with no external
file dependencies at runtime.

## Session model

```go
type Session struct {
    File *gvas.File
}
```

Sessions live in a `map[string]*Session` guarded by a `sync.RWMutex`, keyed
by a random hex token (`crypto/rand`, 16 bytes → 32 hex chars) minted on
upload. The token appears in every subsequent URL for that session
(`/session/{id}/children`, `/session/{id}/edit`, `/session/{id}/download`)
— no cookies, no auth, since this is a local single-user tool.

No TTL/eviction in this pass: sessions live for the server process's
lifetime. Acceptable for a local dev tool; noted as a known limitation
rather than solved now (YAGNI — revisit only if this becomes a
long-running shared deployment, which isn't the goal here).

## Tree-walking helper (`treeview.go`)

Two operations, both built on `gvas.Lookup` plus direct field access on
the `*gvas.Property` it returns (all exported):

- **`childrenOf(f *gvas.File, path string, offset, limit int) (kind string, items []childItem, hasMore bool, err error)`**
  Resolves `path` (empty string means the file's root list) via a
  root-aware wrapper around `gvas.Lookup`, then produces a renderable
  list of children based on what the resolved property holds:
  - `Struct != nil` → each field is a child item (name, type, a short
    value preview for scalars, whether it's itself expandable).
  - `NestedFile != nil` → same, but sourced from `NestedFile.Root`
    (transparent descent, matching how `gvas.Lookup` already treats it).
  - `Array != nil` with `.Structs` → each element is a child item labeled
    by index (`[0]`, `[1]`, ...), expandable.
  - `Array != nil` with a scalar slice (`Ints`/`Floats`/`Doubles`/`Int64s`/
    `Bools`/`Strings`) → children are the individual scalar values,
    **paginated** via `offset`/`limit` (default `limit=100`); `hasMore`
    drives an htmx "load more" trigger appended to the fragment.
  - `Array != nil` with only `Bytes`/`RawElements` (opaque) → reported as
    a single non-expandable "N raw bytes" leaf, no pagination needed.
  - `Native != nil` → rendered as a single leaf showing its decoded
    fields (e.g. `{x, y, z}` for a Vector) — not further expandable,
    since native structs aren't part of the editable property-path space.
  - Otherwise (scalar `Bool`/`Str`/`Int32`/`Int64`/`Float32`/`Float64` or
    `Raw` fallback) → this path is a leaf, not a container; `childrenOf`
    on a leaf path is a caller error (shouldn't happen — the UI only
    requests children for nodes marked expandable).

- **`resolveForEdit(f *gvas.File, path string) (*gvas.Property, error)`**
  Thin wrapper around `gvas.Lookup` used by the edit handler.

`childItem` carries enough for the template to render one row plus (for
expandable items) the `hx-get` URL for its own children:

```go
type childItem struct {
    Path       string // full path to pass to childrenOf/resolveForEdit next
    Label      string // display name: field name, or "[N]" for array index
    Type       string // Property.Type, or a synthetic label for scalar-array elements
    Preview    string // short value string for scalar leaves; empty for containers
    Expandable bool
    Editable   bool   // true only for the six scalar kinds Set* supports
}
```

## HTTP endpoints

| Method + Path | Purpose |
|---|---|
| `GET /` | Page shell (upload form, empty tree container) |
| `POST /upload` | Multipart file upload. Decodes via `gvas.Unmarshal`; on success creates a session and returns the root `children.html.tmpl` fragment (200) wired to that session's ID in every child URL; on decode failure, returns a 422 with an inline error fragment, no session created. Request body capped via `http.MaxBytesReader` (50MB — generously above the ~1MB samples). |
| `GET /session/{id}/children?path=&offset=&limit=` | Returns the `children.html.tmpl` fragment for that path. Unknown session → 404 with a "please re-upload" fragment. |
| `POST /session/{id}/edit` (form fields: `path`, `value`) | Resolves `path`, dispatches to the matching `Set*` based on which typed field is populated (mirrors `saveview set`'s `applyScalarEdit`: `Str`→`SetString`, `Bool`→`ParseBool`+`SetBool`, `Int32`→`ParseInt`+`SetInt32`, etc.). On success, returns the updated `leaf_edit.html.tmpl` fragment showing the new value. On a parse error or a non-editable target, returns 400 with an inline error fragment; the session's tree is left unchanged. |
| `GET /session/{id}/download` | `gvas.Marshal`s the session's current tree and streams it back with `Content-Disposition: attachment; filename="<original-name>-edited.sav"` and `Content-Type: application/octet-stream`. Unknown session → 404. |

## Data flow

1. `GET /` renders the upload form.
2. Upload → `handleUpload` decodes, creates a session, returns the root
   fragment; htmx (`hx-post` on the form, targeting the tree container)
   swaps it into the page.
3. Every row rendered by `children.html.tmpl` that's `Expandable` carries
   `hx-get="/session/{id}/children?path=<Path>" hx-trigger="click"
   hx-target="next .children"` (swap-in-place under that row, collapsed
   again on a second click via htmx's built-in toggle idiom).
4. Every row that's `Editable` renders a small inline form (`hx-post
   .../edit`, `hx-target="closest .leaf-row"`, `hx-swap="outerHTML"`) so
   submitting it in place replaces just that row with the updated value.
5. A persistent "Download" link/button (visible once a session exists)
   points at `/session/{id}/download`.

## Error handling

- Corrupt/invalid upload: `gvas.Unmarshal` error message shown inline on
  the upload form (wrapped, not a raw Go error dump); no session created,
  user can retry.
- Session not found (expired/server restarted since the tab was opened):
  every session-scoped endpoint returns a small fragment/page saying so
  and pointing back at the upload form, not a bare 404/500.
- Edit validation: parse errors (bad int/float/bool syntax) render inline
  next to the field with the original value still shown; the session's
  tree is untouched on failure — same invariant `saveview set` already
  guarantees (`applyScalarEdit` never partially mutates on a parse
  error).
- Attempting to edit a non-scalar path: the UI never renders an edit form
  for a non-`Editable` `childItem`, and `handleEdit` independently checks
  the resolved property's kind before calling any `Set*`, returning 400
  if it's not one of the six supported scalar kinds — defense in depth,
  mirroring `saveview set`'s `default:` error case.
- Oversized upload: rejected by `http.MaxBytesReader` with a clear
  "file too large" message rather than an opaque connection reset.

## Testing

- `net/http/httptest`-based handler tests using the three real `.sav`
  files copied into this project's `testdata/`:
  - Upload each file: assert 200, assert the root fragment contains
    known top-level property names (e.g. `WorldName` for
    `WorldInfo.sav`, `Components` for `Player_Local.sav`).
  - Children endpoint: request a known nested path (e.g.
    `UDSData.CurrentTime` in `WorldInfo.sav`) and assert the fragment
    contains its known child field names (`Month`, `Day`, `TimeOfDay`).
  - Children pagination: request a known large scalar array with
    `limit` smaller than its length, assert the returned item count
    matches `limit` and `hasMore` is reflected in the fragment (a
    "load more" control present); request the next page via `offset`
    and assert the remaining items appear.
  - Edit + download round trip: edit a known scalar (e.g. `WorldName`),
    then hit the download endpoint, re-parse the downloaded bytes with
    `gvas.Unmarshal`, and assert the edited field changed while every
    other top-level property is unchanged — the same invariant
    `saveview set`'s `TestRunSetString`/`TestRunSetNeverTouchesInput`
    already verify at the CLI layer, now verified at the HTTP layer.
  - Edit validation: a bad value (e.g. non-numeric string against an
    `IntProperty`) returns 400 and a subsequent download shows the
    original value unchanged.
  - Unknown session ID on each session-scoped endpoint returns the
    "please re-upload" response, not a panic/500.
- No JS test framework. htmx usage is declarative (attributes on
  server-rendered HTML, no custom client-side logic) — manual
  smoke-testing this in a real browser is proportionate, matching how
  `skyverse-save-tool`'s CLI commands were manually smoke-tested
  alongside their automated tests.

## Out of scope for this pass

- Session persistence/eviction beyond process lifetime.
- Multi-file / directory-based file selection (upload-only, per the
  brainstorming decision).
- Adding/removing properties or array elements — same scope limit as
  `gvas`'s `Marshal`/`Set*` themselves (see
  `skyverse-save-tool`'s design spec, "Out of scope for this pass").
- Editing `MapProperty`/`TextProperty`/native-struct/`Raw`-fallback
  values — none of these are supported edit targets in `gvas` today.
- Authentication/multi-user concerns — local single-user tool.
