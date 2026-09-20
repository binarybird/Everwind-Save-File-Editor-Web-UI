# skyverse-save-web

A standalone web UI for browsing and editing Skyverse `.sav` files,
built on top of the `gvas` library from the sibling
[`skyverse-save-tool`](../skyverse-save-tool) project. See
`docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md` for the
design.

## Requirements

This module depends on `skyversesave` via a local `replace` directive
in `go.mod` pointing at wherever `skyverse-save-tool` is checked out on
disk. Currently that line reads:

    replace skyversesave => /home/binarybird/Desktop/analysis/skyverse-save-tool

If you check out `skyverse-save-tool` at a different path (or on a
different machine), update that `replace` line in `go.mod` to match.

## Build and run

    go build ./...
    ./skyverseweb -addr :8080

Then open http://localhost:8080 in a browser, upload a `.sav` file,
browse the property tree (click a node to expand it) or the Inventory
tab's icon grid, edit a value, then download the result. Two downloads
are offered: "Download edited save" (the `.sav` itself) and "Download
.meta" (its checksum sidecar). **The game checks the `.meta` sidecar
against its `.sav` on load and silently reverts to its own backup if
they don't match** — download both and place them together (with their
original names, which this tool preserves) in the save directory, or
the game will discard your edit without any visible error.

Everything (including the vendored htmx script) is embedded in the
binary — no internet access is required at runtime.

## Testing

    go test ./...

Uses the same three real save files as `skyverse-save-tool`'s test
suite (copied into `testdata/`), including a full upload → browse →
edit → download round trip per file.
