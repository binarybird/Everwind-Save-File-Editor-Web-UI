# skyverse-save-web

A standalone web UI for browsing and editing Skyverse `.sav` files,
built on top of the `gvas` library from the sibling
[`skyverse-save-tool`](../skyverse-save-tool) project. See
`docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md` for the
design.

## Requirements

This module depends on `skyversesave` via a local `replace` directive
pointing at `../skyverse-save-tool` — both projects must be checked out
as siblings on disk.

## Build and run

    go build ./...
    ./skyverseweb -addr :8080

Then open http://localhost:8080 in a browser, upload a `.sav` file,
browse the property tree (click a node to expand it), edit a scalar
value inline and press Save, then click "Download edited save" to get
the result.

Everything (including the vendored htmx script) is embedded in the
binary — no internet access is required at runtime.

## Testing

    go test ./...

Uses the same three real save files as `skyverse-save-tool`'s test
suite (copied into `testdata/`), including a full upload → browse →
edit → download round trip per file.
