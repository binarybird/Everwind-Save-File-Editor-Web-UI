# tools/extracticons

One-time maintenance tool: extracts real item icon art from the installed
game's cooked assets into `static/icons/*.png`, and rewrites
`static/items.json` to reference them. See
`docs/superpowers/specs/2026-09-19-inventory-tab-design.md`, "Icon
extraction pipeline".

Not run by the server or by CI — a developer runs it by hand whenever the
game's item catalog changes.

## Getting `retoc`

This tool shells out to `retoc` (https://github.com/trumank/retoc), which
is **not** committed to this repo (it's a ~93MB prebuilt binary — checking
it into git history would bloat every future clone permanently). Download
a Linux build from that project's releases and place it at
`tools/extracticons/retoc` (or pass a different path via `-retoc`):

```bash
# example — check trumank/retoc's releases page for the current asset name
curl -L -o tools/extracticons/retoc <release-asset-url>
chmod +x tools/extracticons/retoc
```

On first run, `retoc` self-extracts a required Oodle codec shared
library (`liboo2corelinux64.so.9`) alongside itself — that's expected and
also not committed (both are gitignored).

## Running

```bash
go run ./tools/extracticons
```

Flags (all optional, see `-help` for current defaults): `-paks` (the
game's `Content/Paks` directory), `-retoc` (path to the binary above),
`-items` (existing `static/items.json` to read and rewrite),
`-icons-out` (output directory for PNGs), `-work` (scratch directory for
`retoc`'s intermediate legacy-format output).

Partial coverage is expected and fine — not every catalog item has a
distinct icon asset, and compressed (non-`PF_B8G8R8A8`) textures are
intentionally unsupported. Items with no extracted icon fall back to
`static/icons/_placeholder.png` at render time.
