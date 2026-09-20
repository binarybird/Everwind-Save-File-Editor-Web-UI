# skyverse-save-web — Inventory tab design

Status: approved by user 2026-09-19 (icon-grid + equipment scope, one-time
offline icon extraction, click-to-edit with add/remove support using a
best-effort clone-or-template strategy for new items — all confirmed
during brainstorming).

## Background

`skyverse-save-web` (this project) currently shows a save file as a
lazily-expanding property tree (the "Tree" tab, existing behavior — see
`docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md`). The
user wants a second tab that renders the player's inventory the way the
game itself does: a grid of item icons in their actual slot positions,
editable by clicking a slot.

A reference screenshot of the in-game inventory screen
(`/home/binarybird/Downloads/a-character-in-armor-in-the-inventory-screen-in-everwind.avif`)
shows three visually distinct regions: a 9-wide inventory grid plus a
9-slot hotbar row, an 8-slot equipment panel, and a 3D character model.
The 3D model is out of reach for a web app (it needs the game's own
model/rigging system) and is excluded; the icon-grid parts are fully
buildable and are this feature's scope.

## Confirmed save-data mapping

Verified directly against `testdata/Player_Local.sav` using the existing
`saveview dump` output (see investigation in this session — not
re-derived here, just recorded):

- **Inventory component** (`ComponentName == "Inventory"`): its
  `Data.Slots` is a flat `ArrayProperty<StructProperty>` of exactly 63
  elements. Indices `0..53` render as the 9×6 backpack grid (row-major:
  `row = index / 9`, `col = index % 9`); indices `54..62` render as the
  9-slot hotbar row below it. This matches the reference screenshot's
  layout exactly (6 grid rows + a visually separated hotbar row = 7 rows
  × 9 columns = 63).
- **Equipment component** (`ComponentName == "Equipment"`): its
  `Data.Slots` is a fixed 8-element array in a stable, confirmed order:
  `[0]=Helmet, [1]=Chestplate, [2]=Gloves, [3]=Boots, [4]=Shield,
  [5]=Necklace, [6]=Ring, [7]=Glider`. This is positional, not
  name-tagged — the design relies on this fixed order holding for every
  save file (consistent with it being a fixed-size struct array in the
  game's own component schema).
- **Each slot element** (`Slots[i]`) has the shape:
  ```
  Items: ArrayProperty<StructProperty>   // 0 or 1 elements in practice
    [0]
      BaseData: ObjectProperty            // "<PackagePath>.<AssetName>" item reference, or "" empty
      Level, Durability, Energy: IntProperty (Energy = -1 when unused)
      bHasDurability: BoolProperty
      Fuel: FloatProperty (-1 when unused)
      FuelType, SpecialEffect: ObjectProperty
      SpecialEffectLevel: IntProperty
      CustomModificators: ArrayProperty<StructProperty>  // stat rolls, empty for plain items
      AdditionalAlchemyEffectsList, AdditionalAlchemyEffectsTiers: ArrayProperty
      UniqueID: StrProperty
  SlotsWithItems: IntProperty            // despite the name, this is the STACK QUANTITY shown as the
                                          // in-game badge number, not a count of slots
  MaxStackCount: IntProperty              // stacking cap (e.g. 200), not itself edited by this feature
  ```
  A slot is "empty" when `Items` has 0 elements (`SlotsWithItems`/
  `MaxStackCount` are still present with placeholder values).

## Icon extraction pipeline (one-time, offline)

The compiled game (and the `retoc` IoStore-extraction tool used to reach
its assets) will not be present wherever this web app runs, so extraction
cannot happen at request time or even at server startup — it is a
maintenance-time step whose *output* (PNG files) ships with the app.

**Tool**: a new, small standalone Go program,
`skyverse-save-web/tools/extracticons/main.go`. Not part of the served
application and not run in CI — a developer runs it manually whenever the
item catalog changes (matching how `static/items.json` was itself
generated). Inputs: a `retoc`-converted legacy `.uexp` file per icon
(the developer runs `retoc to-legacy` against the game's `Content/Paks/`
directory ahead of time, same as this session's investigation) and the
existing `static/items.json` catalog (name/category/object-path triples).

**Per-icon decode** (verified in this session against two icons of
different categories, both 32×32):
1. Locate the pixel-format `FString` (int32 length prefix + ASCII bytes,
   e.g. `PF_B8G8R8A8`) inside the `.uexp` bytes.
2. Read `SizeX`/`SizeY` from the actual per-mip header fields rather than
   assuming 32×32 — the two sampled icons were both 32×32, but this is
   not asserted to hold for every icon in the game, so the decoder must
   read real dimensions, not hardcode them.
3. Skip past the full mip-chain header block to the first (largest) mip's
   raw pixel bytes, and interpret `SizeX * SizeY * 4` bytes as BGRA8,
   converting to a standard PNG via Go's `image`/`image/png` packages.
4. Any `.uexp` that doesn't decode cleanly (unexpected pixel format,
   dimension mismatch, truncated data) is skipped with a warning printed
   to stderr, not a fatal error — the pipeline should extract everything
   it can and report the rest, since coverage will never be literally
   100% (some catalog entries have no matching icon asset at all).

**Output**: `skyverse-save-web/static/icons/<slug>.png` (one file per
successfully-decoded icon, filename derived from the item's asset name)
plus an updated `static/items.json` with a fourth field per entry: the
icon filename, or `""` when no icon was found. A single
`static/icons/_placeholder.png` (checked in, hand-picked generic icon)
covers any item with no matching icon file at render time. All of
`static/icons/*.png` ship via the existing `//go:embed static` directive
— no new embed wiring needed.

## New gvas primitives (touches `skyverse-save-tool`)

The existing `gvas` `Set*` functions only ever change a scalar value at
an existing path — they cannot add or remove elements from a
`StructProperty` array. Add/remove support for empty/occupied inventory
slots needs exactly two new, narrowly-scoped primitives in
`skyverse-save-tool`'s `gvas` package (this is a deliberate, scoped
exception to the "Adding/removing properties or array elements" item in
that project's own out-of-scope list — general property/array mutation
remains out of scope; only struct-array element insert/remove is added,
because this feature needs it):

```go
// AppendStructElement appends elem as a new last element of the
// StructProperty array at path. Returns an error if the resolved
// property isn't a StructProperty-inner ArrayProperty.
func AppendStructElement(f *File, path string, elem []*Property) error

// RemoveStructElement removes the element at index from the
// StructProperty array at path. Returns an error if the resolved
// property isn't a StructProperty-inner ArrayProperty, or index is out
// of range.
func RemoveStructElement(f *File, path string, index int) error
```

Both operate purely on `ArrayValue.Structs` (`[][]*Property`); no other
`ArrayValue` field needs touching, since (confirmed by reading
`encode.go`) the encoded element count is derived from `len(a.Structs)`
directly — there is no separate stored count to keep in sync.

Each gets a unit test in `gvas/structarray_test.go` mirroring the
existing `Set*` round-trip tests: append/remove against a real slot's
`Items` array in `testdata/Player_Local.sav`, `Marshal` the result, and
confirm every other byte in the file is unchanged except the edited
array (same invariant already proven for scalar edits).

## Best-effort item construction (in `skyverse-save-web`, not `gvas`)

Adding a new item to an empty slot needs a full item property list, not
just a `BaseData` value. Building one correctly (real durability, stat
rolls, fuel state) would require parsing the game's Item Data Asset
files — out of reach for this feature (a much larger, separate
investigation). Instead, a deep-copy-based "best effort" strategy,
implemented entirely with `gvas`'s existing exported types (no `gvas`
changes needed for this part):

1. Search every `Items[0]` across both the Inventory and Equipment
   components' `Slots` arrays in the *current* session's file for one
   whose `BaseData` matches the item being added.
2. If found, deep-copy that element's full `[]*Property` (a generic
   recursive copy over `Property`/`ArrayValue`/`Struct`/`Native`/
   `NestedFile` — no per-field special-casing needed, since copying is
   structural, not semantic) and use it as the new element, except
   `UniqueID` is reset to `""` (per-instance identifiers should not be
   duplicated).
3. If no existing instance is found anywhere in the save, fall back to a
   fixed generic template matching what every plain stackable
   resource/consumable in the sample save already looks like: `Level=0,
   Durability=1, bHasDurability=false, Energy=-1, FuelType="",
   Fuel=-1, SpecialEffect="", SpecialEffectLevel=1,
   CustomModificators=[], AdditionalAlchemyEffectsList=[],
   AdditionalAlchemyEffectsTiers=[], UniqueID=""`, with `BaseData` set to
   the chosen item.
4. The UI surfaces a one-line caveat near the item picker on the
   Inventory tab: newly-added equipment (weapons/wearables) may lack
   durability or stat rolls unless a matching item already exists
   elsewhere in the save to clone from.

Removing an item from a slot is simpler and needs no template: it is
just `RemoveStructElement` on that slot's `Items` array at index 0.

## New components in `skyverse-save-web`

```
inventoryview.go       Builds the render model for both grids from a
                        *gvas.File: locates the Inventory/Equipment
                        components (by ComponentName, same pattern
                        treeview.go already uses for component lookup),
                        walks their Slots arrays, and produces:

                        type slotView struct {
                            ArrayPath  string // path to this slot's Items array, e.g.
                                               // `Components[N].Data.Data.Slots[i].Items`
                            ItemPath   string // path to Items[0].BaseData, "" if empty
                            QtyPath    string // path to this slot's SlotsWithItems
                            Occupied   bool
                            IconFile   string // "" -> placeholder
                            Name       string // display name, "" if empty
                            Quantity   int32
                        }

                        type InventoryGridView struct {
                            SessionID string
                            Backpack  []slotView // 54 elements
                            Hotbar    []slotView // 9 elements
                            Equipment []slotView // 8 elements, fixed labeled order
                        }

templates/
  inventory.html.tmpl   Renders InventoryGridView as three CSS-grid
                         sections (backpack, hotbar, equipment). Each
                         occupied slot: <img src="/static/icons/{{.IconFile}}">
                         plus a quantity badge when Quantity > 1, wrapped
                         so clicking opens the existing item-picker
                         widget (reused from the Tree tab) targeting
                         ItemPath, and a small number input targeting
                         QtyPath (reuses the existing generic /edit
                         endpoint - IntProperty is already a supported
                         Set* target, no backend change needed for
                         quantity). Each empty slot: a bare square with
                         a "+" affordance that opens the item picker in
                         "add" mode instead (posts to /slot/add).

static/icons/*.png      Extracted icon art (see pipeline above),
                         embedded via the existing go:embed static
                         directive.
```

Reused as-is, no changes needed: `itempicker.js`/`items.json` (item
search widget), the existing `/session/{id}/edit` endpoint (quantity
edits and occupied-slot item swaps both already fit its
resolve-path-then-`Set*` shape — an item swap is just `SetString` on the
`BaseData` `ObjectProperty`, already supported since the earlier
ObjectProperty work).

## New HTTP endpoints

| Method + Path | Purpose |
|---|---|
| `GET /session/{id}/inventory` | Returns the `inventory.html.tmpl` fragment (the Inventory tab's content) for the session's current file state. |
| `POST /session/{id}/slot/add` (form fields: `arrayPath`, `itemPath`) | Runs the best-effort construction strategy above, then `gvas.AppendStructElement(f, arrayPath, elem)`. Returns the updated single-slot fragment (icon+badge) on success, or a 400 error fragment (e.g. unknown `itemPath`) leaving the session unchanged. |
| `POST /session/{id}/slot/remove` (form field: `arrayPath`) | `gvas.RemoveStructElement(f, arrayPath, 0)`. Returns the updated (now-empty) single-slot fragment. 400 if the slot was already empty. |

## Tab switching

`index.html.tmpl` gains a small tab bar above the content area:
`Tree | Inventory`. Both tabs are plain htmx-driven links
(`hx-get="/session/{id}/children?path=" ` /
`hx-get="/session/{id}/inventory"`, both `hx-target="#tab-content"
hx-swap="innerHTML"`) — no client-side routing, no page reload, and both
tabs operate on the same session so edits made in one are reflected if
you switch to the other and back (a fresh `hx-get` re-renders from the
session's current `*gvas.File` state every time).

## Error handling

- Same session-not-found handling as the existing endpoints (a small
  "please re-upload" fragment, not a bare 404).
- `slot/add` with an `itemPath` not present in `items.json` (tampered
  request, not reachable through the normal picker UI): 400, session
  unchanged.
- `slot/add` on an already-occupied slot, or `slot/remove` on an already
  empty one: 400 with a clear message ("slot already has an item" /
  "slot is already empty") — defense in depth, since the UI itself won't
  normally offer the wrong action for a slot's current state.
- Icon lookup miss (`IconFile == ""`, or an item with no catalog entry at
  all — shouldn't happen since every `BaseData` on the grid came from the
  save file, but the save could reference an item removed in a newer
  game version): renders `_placeholder.png`, never a broken `<img>`.

## Testing

- `gvas/structarray_test.go` (new, in `skyverse-save-tool`): append/remove
  round-trip tests as described above.
- `inventoryview_test.go` (new, in `skyverse-save-web`): build a grid view
  from `testdata/Player_Local.sav` and assert known facts already
  confirmed in this session — slot 0 of the backpack is the RepairKit
  with quantity 57, equipment slot 0 is the Helmet, slot index 54 starts
  the hotbar section, an empty slot (e.g. backpack index 62) renders
  `Occupied=false`.
- `handlers_test.go` additions: `GET .../inventory` returns the fragment
  with expected slot content; `POST .../slot/add` on an empty slot then
  `GET .../download` + re-parse confirms the new item round-trips;
  `POST .../slot/remove` on an occupied slot confirms the item is gone
  and every other byte of the file is unchanged (same
  edit-round-trip-invariant test shape already used for scalar edits).
- Manual smoke test in a real browser (same rationale as the existing
  spec: htmx usage here is declarative, no custom client JS logic beyond
  the already-existing, already-tested item-picker widget) — visually
  confirm the grid layout matches the reference screenshot's slot
  positions and that clicking through add/remove/swap/quantity-edit all
  work end to end.

## Out of scope for this pass

- The 3D character render.
- General property/array add-remove (only struct-array element
  insert/remove for `Items` arrays, as specified above).
- Correct default stats for newly-added equipment items with no existing
  instance to clone from (best-effort template only, documented caveat).
- Drag-and-drop slot reordering/moving items between slots (this feature
  only supports swap-in-place, add, remove, and quantity edit per slot).
- Recipes/crafting, quests, or any other save-data domain — inventory and
  equipment only.
