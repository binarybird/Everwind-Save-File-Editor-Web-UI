# Inventory Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a second "Inventory" tab to skyverse-save-web that renders the player's backpack/hotbar/equipment as an in-game-accurate icon grid, with click-to-edit item swap, quantity edit, remove, and add-to-empty-slot.

**Architecture:** A one-time offline tool extracts real item icons from the game's own cooked assets into embedded PNGs. A pure Go decoder (`iconextract`) turns a texture asset's raw bytes into an image; the extraction tool orchestrates `retoc` + that decoder to populate `static/icons/` and extend `static/items.json`. Server-side, `inventoryview.go` walks the save's `Inventory`/`Equipment` components into a render model; `inventory.html.tmpl` renders it as a CSS grid, reusing the existing item-picker widget and edit endpoint wherever possible. Two new methods on `gvas.Property` (in the sibling `skyverse-save-tool` repo) add struct-array element insert/remove, needed for add/remove-item support.

**Tech Stack:** Go, `html/template`, htmx (existing, vendored), `image`/`image/png` (stdlib), `retoc` (external binary, maintenance-time only).

**Spec:** `docs/superpowers/specs/2026-09-19-inventory-tab-design.md`

## Global Constraints

- The icon-extraction tool (`tools/extracticons`) and its output are a one-time maintenance step. The served application never invokes `retoc` or touches the game install at runtime — only pre-extracted PNGs under `static/icons/`, embedded via the existing `//go:embed static` directive.
- Equipment `Slots[0..7]` order is fixed: `Helmet, Chestplate, Gloves, Boots, Shield, Necklace, Ring, Glider`.
- Inventory `Slots[0..62]`: indices `0..53` render as the 9-wide/6-row backpack grid, `54..62` as the 9-slot hotbar row.
- `SlotsWithItems` is the stack quantity, not a slot count.
- New `gvas` methods are scoped narrowly to `StructProperty`-inner array append/remove — no other array-mutation capability is added.
- Editing (item swap, quantity, remove, add) only ever targets an item's own `Items` array element at index 0; a slot never holds more than one `Items` element in practice.
- **Task 1 lives in a different git repository** (`/home/binarybird/Desktop/analysis/skyverse-save-tool`, sibling to this one) from every other task (`/home/binarybird/Desktop/analysis/skyverse-save-web`). Its dispatch, worktree, and commit all happen in that other repo — flag this explicitly at execution time.

---

### Task 1: `gvas` struct-array append/remove primitives

**Repo:** `/home/binarybird/Desktop/analysis/skyverse-save-tool` (NOT this repo — see Global Constraints).

**Files:**
- Create: `gvas/structarray.go`
- Create: `gvas/structarray_test.go`

**Interfaces:**
- Produces: `func (p *Property) AppendStructElement(elem []*Property) error` and `func (p *Property) RemoveStructElement(index int) error`, both methods on the existing exported `gvas.Property` type. Consumed later by `skyverse-save-web`'s Task 8 as `itemsProp.AppendStructElement(...)` / `itemsProp.RemoveStructElement(0)`, where `itemsProp` comes from `gvas.Lookup(f, someItemsArrayPath)`.

- [ ] **Step 1: Write the failing tests**

```go
// gvas/structarray_test.go
package gvas

import "testing"

// findNamedStructArray recursively searches props for the first property
// named name whose Array is a StructProperty-inner array with exactly
// wantLen elements -- used to locate a real, representative "Items" array
// (an inventory slot) in testdata without hardcoding its exact path.
func findNamedStructArray(props []*Property, name string, wantLen int) *Property {
	for _, p := range props {
		if p.Name == name && p.Array != nil && p.Array.InnerType.Value == "StructProperty" && len(p.Array.Structs) == wantLen {
			return p
		}
		if p.Struct != nil {
			if found := findNamedStructArray(p.Struct, name, wantLen); found != nil {
				return found
			}
		}
		if p.NestedFile != nil {
			if found := findNamedStructArray(p.NestedFile.Root, name, wantLen); found != nil {
				return found
			}
		}
		if p.Array != nil {
			for _, elem := range p.Array.Structs {
				if found := findNamedStructArray(elem, name, wantLen); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

func TestAppendStructElementRoundTrip(t *testing.T) {
	original := readTestdata(t, "Player_Local.sav")
	f, err := Unmarshal(original)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	target := findNamedStructArray(f.Root, "Items", 0)
	if target == nil {
		t.Fatal("no empty \"Items\" struct array found in Player_Local.sav")
	}

	newPath := "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"
	elem := []*Property{
		{Name: "BaseData", Type: "ObjectProperty", Str: &newPath},
	}
	if err := target.AppendStructElement(elem); err != nil {
		t.Fatalf("AppendStructElement: %v", err)
	}
	if len(target.Array.Structs) != 1 {
		t.Fatalf("len(Structs) = %d, want 1 after append", len(target.Array.Structs))
	}

	out, err := Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(out) == len(original) {
		t.Error("expected file length to change after appending an element")
	}

	f2, err := Unmarshal(out)
	if err != nil {
		t.Fatalf("re-Unmarshal: %v", err)
	}
	got := findNamedStructArray(f2.Root, "Items", 1)
	if got == nil {
		t.Fatal("no 1-element \"Items\" struct array found after round-trip")
	}
	base := findProp(got.Array.Structs[0], "BaseData")
	if base == nil || base.Str == nil || *base.Str != newPath {
		t.Fatalf("appended element's BaseData = %+v, want %q", base, newPath)
	}

	island := findProp(f2.Root, "IslandID")
	wantIsland := findProp(f.Root, "IslandID")
	if island == nil || wantIsland == nil || *island.Str != *wantIsland.Str {
		t.Errorf("IslandID changed unexpectedly: got %+v, want %+v", island, wantIsland)
	}
}

func TestRemoveStructElementRoundTrip(t *testing.T) {
	original := readTestdata(t, "Player_Local.sav")
	f, err := Unmarshal(original)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	target := findNamedStructArray(f.Root, "Items", 1)
	if target == nil {
		t.Fatal("no 1-element \"Items\" struct array found in Player_Local.sav")
	}

	if err := target.RemoveStructElement(0); err != nil {
		t.Fatalf("RemoveStructElement: %v", err)
	}
	if len(target.Array.Structs) != 0 {
		t.Fatalf("len(Structs) = %d, want 0 after remove", len(target.Array.Structs))
	}

	out, err := Marshal(f)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(out) == len(original) {
		t.Error("expected file length to change after removing an element")
	}

	f2, err := Unmarshal(out)
	if err != nil {
		t.Fatalf("re-Unmarshal: %v", err)
	}
	island := findProp(f2.Root, "IslandID")
	wantIsland := findProp(f.Root, "IslandID")
	if island == nil || wantIsland == nil || *island.Str != *wantIsland.Str {
		t.Errorf("IslandID changed unexpectedly: got %+v, want %+v", island, wantIsland)
	}
}

func TestStructArrayMethodsRejectNonStructArray(t *testing.T) {
	scalar := int32(5)
	p := &Property{Name: "NotAnArray", Type: "IntProperty", Int32: &scalar}
	if err := p.AppendStructElement(nil); err == nil {
		t.Error("AppendStructElement on a non-array property: expected an error")
	}
	if err := p.RemoveStructElement(0); err == nil {
		t.Error("RemoveStructElement on a non-array property: expected an error")
	}
}

func TestRemoveStructElementRejectsOutOfRange(t *testing.T) {
	f, err := Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	target := findNamedStructArray(f.Root, "Items", 1)
	if target == nil {
		t.Fatal("no 1-element \"Items\" struct array found")
	}
	if err := target.RemoveStructElement(5); err == nil {
		t.Error("RemoveStructElement(5) on a 1-element array: expected an error")
	}
	if err := target.RemoveStructElement(-1); err == nil {
		t.Error("RemoveStructElement(-1): expected an error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/binarybird/Desktop/analysis/skyverse-save-tool && go test ./gvas/... -run 'StructElement' -v`
Expected: FAIL with `p.AppendStructElement undefined` / `p.RemoveStructElement undefined`.

- [ ] **Step 3: Implement the two methods**

```go
// gvas/structarray.go
package gvas

import "fmt"

// AppendStructElement appends elem as a new last element of p's
// StructProperty-inner array (an ArrayProperty or SetProperty whose
// Extra.InnerType.Value == "StructProperty"). Returns an error if p is
// not such an array. The encoded element count is derived from
// len(p.Array.Structs) at Marshal time, so no other bookkeeping is
// needed.
func (p *Property) AppendStructElement(elem []*Property) error {
	if p.Array == nil || p.Array.InnerType.Value != "StructProperty" {
		return fmt.Errorf("gvas: %s is not a struct-array property", p.Name)
	}
	p.Array.Structs = append(p.Array.Structs, elem)
	return nil
}

// RemoveStructElement removes the element at index from p's
// StructProperty-inner array. Returns an error if p is not such an
// array, or if index is out of range.
func (p *Property) RemoveStructElement(index int) error {
	if p.Array == nil || p.Array.InnerType.Value != "StructProperty" {
		return fmt.Errorf("gvas: %s is not a struct-array property", p.Name)
	}
	if index < 0 || index >= len(p.Array.Structs) {
		return fmt.Errorf("gvas: %s: index %d out of range (%d elements)", p.Name, index, len(p.Array.Structs))
	}
	p.Array.Structs = append(p.Array.Structs[:index], p.Array.Structs[index+1:]...)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/binarybird/Desktop/analysis/skyverse-save-tool && go test ./gvas/... -run 'StructElement' -v`
Expected: PASS (all 4 new tests).

- [ ] **Step 5: Run the full gvas test suite to confirm no regressions**

Run: `cd /home/binarybird/Desktop/analysis/skyverse-save-tool && go test ./... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/binarybird/Desktop/analysis/skyverse-save-tool
git add gvas/structarray.go gvas/structarray_test.go
git commit -m "feat(gvas): add AppendStructElement/RemoveStructElement for struct-array editing"
```

---

### Task 2: Item catalog loader

**Repo:** `/home/binarybird/Desktop/analysis/skyverse-save-web`.

**Files:**
- Create: `itemcatalog.go`
- Create: `itemcatalog_test.go`

**Interfaces:**
- Produces: `type catalogEntry struct { Name, IconFile string }` and `func loadItemCatalog(data []byte) (map[string]catalogEntry, error)`, keyed by an item's full object-path string (the same string a `BaseData` `ObjectProperty` holds, and `static/items.json`'s third field). Consumed by Task 3 (as a parameter, via synthetic maps in its own tests) and Task 6 (loads the real embedded `static/items.json` at server startup and passes the result into `NewServer`).

- [ ] **Step 1: Write the failing tests**

```go
// itemcatalog_test.go
package main

import "testing"

func TestLoadItemCatalogThreeFieldFormat(t *testing.T) {
	data := []byte(`[["2553_IDA_RepairKit","Resources_2500-2999","/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"]]`)
	catalog, err := loadItemCatalog(data)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	entry, ok := catalog["/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"]
	if !ok {
		t.Fatal("expected a catalog entry for RepairKit's object path")
	}
	if entry.Name != "2553_IDA_RepairKit" {
		t.Errorf("Name = %q, want %q", entry.Name, "2553_IDA_RepairKit")
	}
	if entry.IconFile != "" {
		t.Errorf("IconFile = %q, want empty (no 4th field present in this format)", entry.IconFile)
	}
}

func TestLoadItemCatalogFourFieldFormat(t *testing.T) {
	data := []byte(`[["2553_IDA_RepairKit","Resources_2500-2999","/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit","2553_IDA_RepairKit.png"]]`)
	catalog, err := loadItemCatalog(data)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	entry := catalog["/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"]
	if entry.IconFile != "2553_IDA_RepairKit.png" {
		t.Errorf("IconFile = %q, want %q", entry.IconFile, "2553_IDA_RepairKit.png")
	}
}

func TestLoadItemCatalogSkipsMalformedEntries(t *testing.T) {
	data := []byte(`[["OnlyOneField"],["Name","Category","/Game/Path.Path"]]`)
	catalog, err := loadItemCatalog(data)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("len(catalog) = %d, want 1 (the malformed entry should be skipped)", len(catalog))
	}
}

func TestLoadItemCatalogRejectsInvalidJSON(t *testing.T) {
	if _, err := loadItemCatalog([]byte(`not json`)); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestLoadItemCatalog -v`
Expected: FAIL with `undefined: loadItemCatalog` / `undefined: catalogEntry`.

- [ ] **Step 3: Implement**

```go
// itemcatalog.go
package main

import "encoding/json"

// catalogEntry is one item's display metadata, looked up by its full
// gvas object-path string (the same string stored in a BaseData
// ObjectProperty, and used as static/items.json's third field).
type catalogEntry struct {
	Name     string
	IconFile string // "" if no icon has been extracted for this item
}

// loadItemCatalog parses static/items.json's format: a JSON array of
// [name, category, objectPath] triples (the format used by the existing
// Tree-tab item picker), or [name, category, objectPath, iconFile] once
// tools/extracticons (Task 5) has run. The fourth field is read
// defensively -- entries may still have only 3 fields in a checkout
// where the icon pipeline hasn't been run yet -- so this never panics on
// the older format. Entries with fewer than 3 fields are skipped.
func loadItemCatalog(data []byte) (map[string]catalogEntry, error) {
	var raw [][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	catalog := make(map[string]catalogEntry, len(raw))
	for _, entry := range raw {
		if len(entry) < 3 {
			continue
		}
		name, objectPath := entry[0], entry[2]
		iconFile := ""
		if len(entry) > 3 {
			iconFile = entry[3]
		}
		catalog[objectPath] = catalogEntry{Name: name, IconFile: iconFile}
	}
	return catalog, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestLoadItemCatalog -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add itemcatalog.go itemcatalog_test.go
git commit -m "feat: add item catalog loader for icon/name lookup"
```

---

### Task 3: Inventory grid view model

**Files:**
- Create: `inventoryview.go`
- Create: `inventoryview_test.go`

**Interfaces:**
- Consumes: `catalogEntry`, `map[string]catalogEntry` (Task 2). `arrayLen(*gvas.ArrayValue) int` and `rowID(string) string` (already in `treeview.go`, same package).
- Produces:
  ```go
  type slotView struct {
      SessionID  string
      SlotPath   string
      ArrayPath  string
      ItemPath   string
      QtyPath    string
      RowID      string
      Label      string
      Occupied   bool
      IconFile   string
      Name       string
      ObjectPath string
      Quantity   int32
      Error      string
  }
  type InventoryGridView struct {
      SessionID string
      Backpack  []slotView // 54 elements
      Hotbar    []slotView // 9 elements
      Equipment []slotView // 8 elements
  }
  func BuildInventoryGridView(f *gvas.File, sessionID string, catalog map[string]catalogEntry) (*InventoryGridView, error)
  func buildSlotView(slotProps []*gvas.Property, slotPath, sessionID string, catalog map[string]catalogEntry) slotView
  func componentSlotsPath(f *gvas.File, componentName string) (string, error)
  func findFieldProp(props []*gvas.Property, name string) *gvas.Property
  ```
  `BuildInventoryGridView`/`buildSlotView`/`componentSlotsPath`/`findFieldProp` are consumed directly by Task 6 (handler), Task 7 (re-render after edit), and Task 8 (best-effort item construction, remove/add handlers).

- [ ] **Step 1: Write the failing tests**

```go
// inventoryview_test.go
package main

import (
	"testing"

	"skyversesave/gvas"
)

func TestBuildInventoryGridViewShape(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	view, err := BuildInventoryGridView(f, "sess1", map[string]catalogEntry{})
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	if len(view.Backpack) != 54 {
		t.Errorf("len(Backpack) = %d, want 54", len(view.Backpack))
	}
	if len(view.Hotbar) != 9 {
		t.Errorf("len(Hotbar) = %d, want 9", len(view.Hotbar))
	}
	if len(view.Equipment) != 8 {
		t.Errorf("len(Equipment) = %d, want 8", len(view.Equipment))
	}
}

func TestBuildInventoryGridViewBackpackSlot0IsRepairKit(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	wantPath := "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"
	catalog := map[string]catalogEntry{wantPath: {Name: "Repair Kit", IconFile: "repairkit.png"}}
	view, err := BuildInventoryGridView(f, "sess1", catalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slot0 := view.Backpack[0]
	if !slot0.Occupied {
		t.Fatal("Backpack[0]: expected Occupied=true")
	}
	if slot0.Name != "Repair Kit" {
		t.Errorf("Backpack[0].Name = %q, want %q", slot0.Name, "Repair Kit")
	}
	if slot0.IconFile != "repairkit.png" {
		t.Errorf("Backpack[0].IconFile = %q, want %q", slot0.IconFile, "repairkit.png")
	}
	if slot0.ObjectPath != wantPath {
		t.Errorf("Backpack[0].ObjectPath = %q, want %q", slot0.ObjectPath, wantPath)
	}
	if slot0.Quantity != 57 {
		t.Errorf("Backpack[0].Quantity = %d, want 57", slot0.Quantity)
	}
	if slot0.ItemPath != slot0.ArrayPath+"[0].BaseData" {
		t.Errorf("Backpack[0].ItemPath = %q, want %q", slot0.ItemPath, slot0.ArrayPath+"[0].BaseData")
	}
}

func TestBuildInventoryGridViewHotbarLastSlotIsEmpty(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	view, err := BuildInventoryGridView(f, "sess1", map[string]catalogEntry{})
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	last := view.Hotbar[8] // Slots[62] in the save, confirmed empty in testdata
	if last.Occupied {
		t.Error("Hotbar[8] (Slots[62]): expected Occupied=false")
	}
	if last.ItemPath != "" {
		t.Errorf("Hotbar[8].ItemPath = %q, want empty for an unoccupied slot", last.ItemPath)
	}
}

func TestBuildInventoryGridViewEquipmentOrderAndLabels(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	view, err := BuildInventoryGridView(f, "sess1", map[string]catalogEntry{})
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	wantLabels := []string{"Helmet", "Chestplate", "Gloves", "Boots", "Shield", "Necklace", "Ring", "Glider"}
	for i, want := range wantLabels {
		if view.Equipment[i].Label != want {
			t.Errorf("Equipment[%d].Label = %q, want %q", i, view.Equipment[i].Label, want)
		}
	}
	helmet := view.Equipment[0]
	if !helmet.Occupied {
		t.Fatal("Equipment[0] (Helmet): expected Occupied=true (testdata has one equipped)")
	}
	wantPath := "/Game/Data/Items/Tools_3000+/Wearables/IDA_3113_00_Helmet_Master_Steel_Wearables.IDA_3113_00_Helmet_Master_Steel_Wearables"
	if helmet.ObjectPath != wantPath {
		t.Errorf("Equipment[0].ObjectPath = %q, want %q", helmet.ObjectPath, wantPath)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestBuildInventoryGridView -v`
Expected: FAIL with `undefined: BuildInventoryGridView`.

- [ ] **Step 3: Implement**

```go
// inventoryview.go
package main

import (
	"fmt"

	"skyversesave/gvas"
)

// slotView is one inventory or equipment slot's render data. See
// docs/superpowers/specs/2026-09-19-inventory-tab-design.md, "New
// components in skyverse-save-web".
type slotView struct {
	SessionID  string
	SlotPath   string // path to this slot itself (parent of Items/SlotsWithItems)
	ArrayPath  string // SlotPath + ".Items"
	ItemPath   string // ArrayPath + "[0].BaseData"; "" if the slot is empty
	QtyPath    string // SlotPath + ".SlotsWithItems"
	RowID      string
	Label      string // equipment slot name ("Helmet" etc.); "" for backpack/hotbar
	Occupied   bool
	IconFile   string // "" -> render the placeholder icon
	Name       string // display name (catalog name, or the raw object path as a fallback)
	ObjectPath string // the raw BaseData value; "" if empty. This, not Name, is the
	                   // item-picker input's value -- Name is display-only.
	Quantity   int32
	Error      string // set only when re-rendering a slot after a failed edit/add/remove
}

// InventoryGridView is what the "inventory" template renders.
type InventoryGridView struct {
	SessionID string
	Backpack  []slotView // 54 elements: the Inventory component's Slots[0..53]
	Hotbar    []slotView // 9 elements: Slots[54..62]
	Equipment []slotView // 8 elements, in equipmentSlotLabels order
}

// equipmentSlotLabels names the Equipment component's Slots[0..7] in
// their confirmed, fixed positional order. See the spec's "Confirmed
// save-data mapping".
var equipmentSlotLabels = [8]string{
	"Helmet", "Chestplate", "Gloves", "Boots",
	"Shield", "Necklace", "Ring", "Glider",
}

// BuildInventoryGridView walks f's Inventory and Equipment components and
// builds the render model for the Inventory tab.
func BuildInventoryGridView(f *gvas.File, sessionID string, catalog map[string]catalogEntry) (*InventoryGridView, error) {
	invSlotsPath, err := componentSlotsPath(f, "Inventory")
	if err != nil {
		return nil, err
	}
	invSlots, err := gvas.Lookup(f, invSlotsPath)
	if err != nil {
		return nil, fmt.Errorf("inventory Slots: %w", err)
	}
	if invSlots.Array == nil || len(invSlots.Array.Structs) != 63 {
		return nil, fmt.Errorf("inventory Slots: expected 63 elements, got %d", arrayLen(invSlots.Array))
	}

	eqSlotsPath, err := componentSlotsPath(f, "Equipment")
	if err != nil {
		return nil, err
	}
	eqSlots, err := gvas.Lookup(f, eqSlotsPath)
	if err != nil {
		return nil, fmt.Errorf("equipment Slots: %w", err)
	}
	if eqSlots.Array == nil || len(eqSlots.Array.Structs) != 8 {
		return nil, fmt.Errorf("equipment Slots: expected 8 elements, got %d", arrayLen(eqSlots.Array))
	}

	view := &InventoryGridView{SessionID: sessionID}
	for i := 0; i < 54; i++ {
		path := fmt.Sprintf("%s[%d]", invSlotsPath, i)
		view.Backpack = append(view.Backpack, buildSlotView(invSlots.Array.Structs[i], path, sessionID, catalog))
	}
	for i := 54; i < 63; i++ {
		path := fmt.Sprintf("%s[%d]", invSlotsPath, i)
		view.Hotbar = append(view.Hotbar, buildSlotView(invSlots.Array.Structs[i], path, sessionID, catalog))
	}
	for i := 0; i < 8; i++ {
		path := fmt.Sprintf("%s[%d]", eqSlotsPath, i)
		sv := buildSlotView(eqSlots.Array.Structs[i], path, sessionID, catalog)
		sv.Label = equipmentSlotLabels[i]
		view.Equipment = append(view.Equipment, sv)
	}
	return view, nil
}

// buildSlotView renders one Slots[i] element given slotProps (its own
// property list) and slotPath (the path that resolves to it, e.g.
// "Components[1].Data.Data.Slots[0]").
func buildSlotView(slotProps []*gvas.Property, slotPath, sessionID string, catalog map[string]catalogEntry) slotView {
	sv := slotView{
		SessionID: sessionID,
		SlotPath:  slotPath,
		ArrayPath: slotPath + ".Items",
		QtyPath:   slotPath + ".SlotsWithItems",
		RowID:     rowID(slotPath),
	}
	if qty := findFieldProp(slotProps, "SlotsWithItems"); qty != nil && qty.Int32 != nil {
		sv.Quantity = *qty.Int32
	}
	items := findFieldProp(slotProps, "Items")
	if items == nil || items.Array == nil || len(items.Array.Structs) == 0 {
		return sv
	}
	sv.Occupied = true
	sv.ItemPath = sv.ArrayPath + "[0].BaseData"
	itemProps := items.Array.Structs[0]
	base := findFieldProp(itemProps, "BaseData")
	if base != nil && base.Str != nil {
		sv.ObjectPath = *base.Str
		if entry, ok := catalog[sv.ObjectPath]; ok {
			sv.Name = entry.Name
			sv.IconFile = entry.IconFile
		} else {
			sv.Name = sv.ObjectPath
		}
	}
	return sv
}

// componentSlotsPath finds the Components[] element whose ComponentName
// equals name and returns the property path to its Data.Data.Slots
// array (the outer "Data" is the component's own struct field; the
// inner "Data" is an ArrayProperty<ByteProperty> that gvas decodes as a
// NestedFile, which gvas.Lookup transparently descends into).
func componentSlotsPath(f *gvas.File, name string) (string, error) {
	comps, err := gvas.Lookup(f, "Components")
	if err != nil {
		return "", fmt.Errorf("no top-level Components array: %w", err)
	}
	if comps.Array == nil || comps.Array.Structs == nil {
		return "", fmt.Errorf("Components property is not a struct array")
	}
	for i, elem := range comps.Array.Structs {
		if n := findFieldProp(elem, "ComponentName"); n != nil && n.Str != nil && *n.Str == name {
			return fmt.Sprintf("Components[%d].Data.Data.Slots", i), nil
		}
	}
	return "", fmt.Errorf("no component named %q", name)
}

// findFieldProp returns the first property named name in props, or nil.
func findFieldProp(props []*gvas.Property, name string) *gvas.Property {
	for _, p := range props {
		if p.Name == name {
			return p
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestBuildInventoryGridView -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite to confirm no regressions**

Run: `go test ./... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add inventoryview.go inventoryview_test.go
git commit -m "feat: add inventory grid view model (backpack/hotbar/equipment)"
```

---

### Task 4: Icon pixel decoder (`iconextract` package)

**Files:**
- Create: `iconextract/decode.go`
- Create: `iconextract/decode_test.go`

**Interfaces:**
- Produces: `func Decode(uexp []byte) (*image.NRGBA, error)`. Consumed by Task 5's extraction tool.

- [ ] **Step 1: Write the failing tests**

```go
// iconextract/decode_test.go
package iconextract

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildFixture constructs a synthetic classic-.uexp-shaped byte buffer:
// [junk][int32 len-prefix]["PF_B8G8R8A8"][null][4 reserved bytes]
// [int32 mip count][pad][mip0 pixel bytes][filler for smaller mips].
// pad's *length* (not content) and filler's *length* both matter -- they
// stand in for the real per-mip header bytes and the smaller mips' pixel
// bytes respectively, which Decode's byte-accounting must skip/ignore
// without needing to know their real internal structure.
func buildFixture(dim int, pad, mip0 []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("junkjunk")

	marker := "PF_B8G8R8A8"
	lenPrefix := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenPrefix, uint32(len(marker)+1))
	buf.Write(lenPrefix)
	buf.WriteString(marker)
	buf.WriteByte(0)

	buf.Write(make([]byte, 4)) // reserved

	mipCount := 0
	for d := dim; d >= 1; d /= 2 {
		mipCount++
	}
	mipCountBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(mipCountBytes, uint32(mipCount))
	buf.Write(mipCountBytes)

	buf.Write(pad)
	buf.Write(mip0)

	// filler bytes standing in for every smaller mip's pixel data, so
	// Decode's total-remaining-minus-total-payload arithmetic works out
	// to exactly len(pad).
	d := dim
	for m := 1; m < mipCount; m++ {
		d /= 2
		buf.Write(make([]byte, d*d*4))
	}
	return buf.Bytes()
}

func TestDecodeExtracts2x2Icon(t *testing.T) {
	pad := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11} // 7 arbitrary header padding bytes
	mip0 := []byte{
		10, 20, 30, 255, // (0,0): B,G,R,A
		40, 50, 60, 255, // (1,0)
		70, 80, 90, 128, // (0,1)
		100, 110, 120, 0, // (1,1)
	}
	fixture := buildFixture(2, pad, mip0)

	img, err := Decode(fixture)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatalf("image size = %dx%d, want 2x2", img.Bounds().Dx(), img.Bounds().Dy())
	}
	c := img.NRGBAAt(0, 0)
	if c.R != 30 || c.G != 20 || c.B != 10 || c.A != 255 {
		t.Errorf("pixel (0,0) = %+v, want R=30 G=20 B=10 A=255", c)
	}
	c = img.NRGBAAt(1, 1)
	if c.R != 120 || c.G != 110 || c.B != 100 || c.A != 0 {
		t.Errorf("pixel (1,1) = %+v, want R=120 G=110 B=100 A=0", c)
	}
}

func TestDecodeRejectsMissingPixelFormatMarker(t *testing.T) {
	fixture := []byte("some random bytes with no recognized pixel format marker anywhere in them at all")
	if _, err := Decode(fixture); err == nil {
		t.Fatal("expected an error when no PF_B8G8R8A8 marker is present")
	}
}

func TestDecodeRejectsTruncatedFile(t *testing.T) {
	pad := []byte{}
	mip0 := []byte{1, 2, 3, 4} // only 1 pixel's worth, but dim=2 needs 4
	fixture := buildFixture(2, pad, mip0)
	fixture = fixture[:len(fixture)-8] // truncate away the last mip's filler AND part of mip0
	if _, err := Decode(fixture); err == nil {
		t.Fatal("expected an error for a truncated file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./iconextract/... -v`
Expected: FAIL with `undefined: Decode`.

- [ ] **Step 3: Implement**

```go
// iconextract/decode.go
package iconextract

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
)

// pixelFormatMarker is the only pixel format this decoder understands --
// UTexture2D's uncompressed 32-bit BGRA layout. A compressed format
// (DXT/BC-family) is reported as an error so callers can skip that
// asset. See docs/superpowers/specs/2026-09-19-inventory-tab-design.md,
// "Icon extraction pipeline" for the derivation of this algorithm.
const pixelFormatMarker = "PF_B8G8R8A8"

// Decode extracts the largest (mip 0) image from a classic-format
// UTexture2D .uexp file's raw bytes.
//
// The pixel format string is followed by a reserved int32 and a mip
// count (int32); a full square power-of-two mip chain down to 1x1 is
// assumed, so dim = 2^(mipCount-1). The exact per-mip header layout
// isn't parsed field-by-field -- instead, the total header-block size is
// derived algebraically: (bytes remaining after the pixel format
// string) minus (the full mip chain's total pixel byte count) equals
// the header block, which precedes all pixel data. This was verified
// against two real icons (different categories, both 32x32) during this
// feature's design spike.
func Decode(uexp []byte) (*image.NRGBA, error) {
	idx := bytes.Index(uexp, []byte(pixelFormatMarker))
	if idx < 0 {
		return nil, fmt.Errorf("iconextract: pixel format %q not found (likely a compressed format, unsupported)", pixelFormatMarker)
	}
	if idx < 4 {
		return nil, fmt.Errorf("iconextract: pixel format string at offset %d has no room for its length prefix", idx)
	}
	lengthPrefix := int32(binary.LittleEndian.Uint32(uexp[idx-4 : idx]))
	if int(lengthPrefix) != len(pixelFormatMarker)+1 {
		return nil, fmt.Errorf("iconextract: unexpected pixel format string length prefix %d, want %d", lengthPrefix, len(pixelFormatMarker)+1)
	}
	after := idx + int(lengthPrefix)
	if after+8 > len(uexp) {
		return nil, fmt.Errorf("iconextract: file truncated after pixel format string")
	}
	mipCount := int32(binary.LittleEndian.Uint32(uexp[after+4 : after+8]))
	if mipCount < 1 || mipCount > 16 {
		return nil, fmt.Errorf("iconextract: implausible mip count %d", mipCount)
	}
	dim := 1 << uint(mipCount-1)

	totalPixelPayload := 0
	for m := int32(0); m < mipCount; m++ {
		mipDim := dim >> uint(m)
		totalPixelPayload += mipDim * mipDim * 4
	}
	totalRemaining := len(uexp) - after
	headerBytes := totalRemaining - totalPixelPayload
	if headerBytes < 0 {
		return nil, fmt.Errorf("iconextract: computed negative header size (%d); mip count %d / dim %d likely wrong for this file", headerBytes, mipCount, dim)
	}
	pixelStart := after + headerBytes
	pixelEnd := pixelStart + dim*dim*4
	if pixelEnd > len(uexp) {
		return nil, fmt.Errorf("iconextract: computed pixel range [%d:%d) exceeds file length %d", pixelStart, pixelEnd, len(uexp))
	}
	bgra := uexp[pixelStart:pixelEnd]

	img := image.NewNRGBA(image.Rect(0, 0, dim, dim))
	for i := 0; i < dim*dim; i++ {
		b, g, r, a := bgra[i*4], bgra[i*4+1], bgra[i*4+2], bgra[i*4+3]
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, a
	}
	return img, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./iconextract/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add iconextract/decode.go iconextract/decode_test.go
git commit -m "feat: add pure-Go UTexture2D BGRA8 icon decoder"
```

---

### Task 5: Icon extraction tool -- build it, run it, commit the output

**Files:**
- Create: `tools/extracticons/main.go`
- Modify: `static/items.json` (regenerated with a 4th field)
- Create: `static/icons/*.png` (one per successfully-decoded item, plus `_placeholder.png`)

**Interfaces:**
- Consumes: `iconextract.Decode` (Task 4).
- Produces: the on-disk `static/icons/` directory and updated `static/items.json`, consumed at runtime by Task 6 via `loadItemCatalog` (Task 2) and the existing `//go:embed static` directive (`main.go`) -- no code-level interface, just data.

This tool is a one-time maintenance script (see Global Constraints) -- it is not covered by `go test ./...` and is not run by CI. It depends on the `retoc` binary and the installed game, both present on this machine:
- `retoc`: `/tmp/claude-1000/-home-binarybird-Desktop-analysis/fe5aa60c-2a41-4ecb-a71f-83be61c7d6ac/scratchpad/retoc_cli-x86_64-unknown-linux-gnu/retoc` (copy it somewhere durable first, e.g. `tools/extracticons/retoc`, since the scratchpad directory is session-scoped and will not survive)
- Game Paks directory: `/home/binarybird/.local/share/Steam/steamapps/common/Everwind/skyverse/Content/Paks`

- [ ] **Step 1: Copy `retoc` somewhere durable**

```bash
mkdir -p tools/extracticons
cp /tmp/claude-1000/-home-binarybird-Desktop-analysis/fe5aa60c-2a41-4ecb-a71f-83be61c7d6ac/scratchpad/retoc_cli-x86_64-unknown-linux-gnu/retoc tools/extracticons/retoc
chmod +x tools/extracticons/retoc
```

If that scratchpad path no longer exists (a new session), download a fresh Linux `retoc` build from its GitHub releases (`trumank/retoc`) to the same destination instead.

- [ ] **Step 2: Verify the bulk-conversion filters against the real container, before writing any Go code**

```bash
PAKS=/home/binarybird/.local/share/Steam/steamapps/common/Everwind/skyverse/Content/Paks
mkdir -p /tmp/extracticons-verify/items /tmp/extracticons-verify/icons
./tools/extracticons/retoc to-legacy -f "Skyverse/Content/Data/Items/" "$PAKS" /tmp/extracticons-verify/items 2>&1 | tail -5
find /tmp/extracticons-verify/items -name "*.uexp" | wc -l
./tools/extracticons/retoc to-legacy -f "Skyverse/Content/Textures/Icons/" "$PAKS" /tmp/extracticons-verify/icons 2>&1 | tail -5
find /tmp/extracticons-verify/icons -name "*.uexp" | wc -l
```

Confirm both counts are in the thousands (matching `static/items.json`'s ~5,289 entries and the ~5,164 icon files found during this feature's design spike). If either filter yields zero or a suspiciously small count, the filter prefix needs adjusting (`retoc to-legacy --help` documents `-f`/`--filter`) -- do that now, empirically, before writing Step 3's code, and use the working prefix there.

- [ ] **Step 3: Implement the orchestration tool**

```go
// tools/extracticons/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"skyverseweb/iconextract"
)

func main() {
	paksDir := flag.String("paks", "/home/binarybird/.local/share/Steam/steamapps/common/Everwind/skyverse/Content/Paks", "game's Paks directory")
	retocPath := flag.String("retoc", "tools/extracticons/retoc", "path to the retoc binary")
	itemsPath := flag.String("items", "static/items.json", "existing item catalog (3-field format) to read and rewrite (4-field)")
	iconsOut := flag.String("icons-out", "static/icons", "directory to write extracted icon PNGs into")
	workDir := flag.String("work", "/tmp/extracticons-work", "scratch directory for retoc's legacy conversion output")
	flag.Parse()

	if err := run(*paksDir, *retocPath, *itemsPath, *iconsOut, *workDir); err != nil {
		log.Fatal(err)
	}
}

type itemEntry struct {
	Name       string
	Category   string
	ObjectPath string
}

func run(paksDir, retocPath, itemsPath, iconsOut, workDir string) error {
	items, err := readItems(itemsPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", itemsPath, err)
	}
	log.Printf("loaded %d items from %s", len(items), itemsPath)

	itemsLegacyDir := filepath.Join(workDir, "items")
	iconsLegacyDir := filepath.Join(workDir, "icons")
	if err := retocToLegacy(retocPath, "Skyverse/Content/Data/Items/", paksDir, itemsLegacyDir); err != nil {
		return fmt.Errorf("bulk-converting item assets: %w", err)
	}
	if err := retocToLegacy(retocPath, "Skyverse/Content/Textures/Icons/", paksDir, iconsLegacyDir); err != nil {
		return fmt.Errorf("bulk-converting icon assets: %w", err)
	}

	iconRefPattern := regexp.MustCompile(`/Game/Textures/Icons/[^\x00]*`)
	if err := os.MkdirAll(iconsOut, 0o755); err != nil {
		return err
	}

	found, missing := 0, 0
	for _, it := range items {
		itemUexp := legacyUexpPath(itemsLegacyDir, it.ObjectPath)
		data, err := os.ReadFile(itemUexp)
		if err != nil {
			log.Printf("skip %s: item asset not converted (%v)", it.ObjectPath, err)
			missing++
			continue
		}
		match := iconRefPattern.Find(data)
		if match == nil {
			log.Printf("skip %s: no icon reference found in its asset", it.ObjectPath)
			missing++
			continue
		}
		iconObjectPath := string(match)
		iconUexp := legacyUexpPath(iconsLegacyDir, iconObjectPath)
		iconData, err := os.ReadFile(iconUexp)
		if err != nil {
			log.Printf("skip %s: referenced icon %s not converted (%v)", it.ObjectPath, iconObjectPath, err)
			missing++
			continue
		}
		img, err := iconextract.Decode(iconData)
		if err != nil {
			log.Printf("skip %s: decoding icon %s: %v", it.ObjectPath, iconObjectPath, err)
			missing++
			continue
		}
		iconFile := slugify(it.Name) + ".png"
		if err := writePNG(filepath.Join(iconsOut, iconFile), img); err != nil {
			return fmt.Errorf("writing %s: %w", iconFile, err)
		}
		found++
	}
	log.Printf("extracted %d icons, %d items had no usable icon", found, missing)

	if err := writePlaceholder(filepath.Join(iconsOut, "_placeholder.png")); err != nil {
		return fmt.Errorf("writing placeholder icon: %w", err)
	}

	return writeItemsWithIcons(itemsPath, items, iconsOut)
}

func readItems(path string) ([]itemEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw [][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	items := make([]itemEntry, 0, len(raw))
	for _, e := range raw {
		if len(e) < 3 {
			continue
		}
		items = append(items, itemEntry{Name: e[0], Category: e[1], ObjectPath: e[2]})
	}
	return items, nil
}

// legacyUexpPath maps a gvas object path ("/Game/Foo/Bar.Bar") to the
// file retoc to-legacy writes it to ("<root>/Skyverse/Content/Foo/Bar.uexp").
func legacyUexpPath(legacyRoot, objectPath string) string {
	packagePath := strings.SplitN(objectPath, ".", 2)[0]
	rel := "Skyverse/Content" + strings.TrimPrefix(packagePath, "/Game") + ".uexp"
	return filepath.Join(legacyRoot, rel)
}

func retocToLegacy(retocPath, filter, paksDir, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(retocPath, "to-legacy", "-f", filter, paksDir, outDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var slugInvalid = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func slugify(name string) string {
	return slugInvalid.ReplaceAllString(name, "_")
}

func writePNG(path string, img *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// writePlaceholder writes a small flat gray-with-alpha square, used for
// any item with no extracted icon.
func writePlaceholder(path string) error {
	return writePNG(path, newPlaceholder(32))
}

// newPlaceholder builds a plain dim x dim semi-transparent gray square.
func newPlaceholder(dim int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, dim, dim))
	for i := 0; i < dim*dim; i++ {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = 128, 128, 128, 180
	}
	return img
}

func writeItemsWithIcons(path string, items []itemEntry, iconsOut string) error {
	out := make([][4]string, len(items))
	for i, it := range items {
		iconFile := ""
		candidate := filepath.Join(iconsOut, slugify(it.Name)+".png")
		if _, err := os.Stat(candidate); err == nil {
			iconFile = slugify(it.Name) + ".png"
		}
		out[i] = [4]string{it.Name, it.Category, it.ObjectPath, iconFile}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: Build the tool**

Run: `go build -o /tmp/extracticons ./tools/extracticons`
Expected: builds with no errors once the Step 3 type mismatch above is resolved.

- [ ] **Step 5: Run it for real**

```bash
go run ./tools/extracticons
```

This takes a while (two full-container `retoc to-legacy` passes plus per-item byte scanning). Expected log output: item/icon counts from Step 2's verification, then a final "extracted N icons, M items had no usable icon" line. Some `missing` count is expected and fine (not every catalog entry has a distinct icon asset) -- the spec's out-of-scope section already accounts for partial coverage.

- [ ] **Step 6: Spot-check the output**

```bash
ls static/icons | wc -l
ls static/icons | head -5
python3 -c "import json; d=json.load(open('static/items.json')); print(len(d)); print(d[0])"
```

Confirm `static/icons/_placeholder.png` exists, a plausible number of PNGs were produced, and `static/items.json` entries now have 4 elements each. Read two or three of the produced PNGs (e.g. the one matching `2553_IDA_RepairKit`) with an image-viewing tool to visually confirm they look like real item icons, not corrupted/blank images.

- [ ] **Step 7: Run the full test suite to confirm no regressions**

Run: `go test ./... -race`
Expected: PASS (this task doesn't touch tested Go packages other than adding an untested `tools/` binary, which `go test ./...` still builds/vets).

- [ ] **Step 8: Commit**

```bash
git add tools/extracticons/main.go tools/extracticons/retoc static/icons static/items.json
git commit -m "feat: extract real item icons from the game's assets"
```

If `static/icons/*.png` totals push the repo size somewhere uncomfortable, note that in the commit message and flag it to the user rather than silently splitting the commit -- don't decide unilaterally to drop icons or change the embedding strategy.

---

### Task 6: Inventory tab -- view-only render

**Files:**
- Modify: `main.go` (load the item catalog at startup, pass it into `NewServer`)
- Modify: `handlers.go` (add `itemCatalog` field, change `NewServer` signature, add `handleInventory` + route)
- Modify: `handlers_test.go` (`newTestServer` must build and pass a real catalog)
- Create: `templates/inventory.html.tmpl`
- Modify: `templates/index.html.tmpl` (tab bar, `uploadResponse` restructuring)
- Modify: `static/style.css` (grid/tab styling)
- Create: `handlers_inventory_test.go`

**Interfaces:**
- Consumes: `loadItemCatalog`/`catalogEntry` (Task 2), `BuildInventoryGridView` (Task 3).
- Produces: `Server.itemCatalog map[string]catalogEntry` field, `NewServer(store *SessionStore, templates *template.Template, itemCatalog map[string]catalogEntry) *Server` (signature change), `GET /session/{id}/inventory` route. Task 7 and Task 8 both call `s.itemCatalog` and add more routes alongside this one.

- [ ] **Step 1: Write the failing test**

```go
// handlers_inventory_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleInventoryRendersGrid(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/inventory", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleInventory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="inventory-grid"`) {
		t.Errorf("expected the inventory grid container, got:\n%s", body)
	}
	if !strings.Contains(body, "/static/icons/") {
		t.Errorf("expected at least one icon <img src=\"/static/icons/...\">, got:\n%s", body)
	}
}

func TestHandleInventoryUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/inventory", nil)
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()

	s.handleInventory(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestHandleInventory -v`
Expected: FAIL to compile (`s.handleInventory undefined`, and `newTestServer`/`NewServer` call-site mismatches once Step 3 changes their signatures -- that's expected mid-task).

- [ ] **Step 3: Update `Server`/`NewServer` and add the handler**

In `handlers.go`, change:

```go
type Server struct {
	store     *SessionStore
	templates *template.Template
}

func NewServer(store *SessionStore, templates *template.Template) *Server {
	return &Server{store: store, templates: templates}
}
```

to:

```go
type Server struct {
	store       *SessionStore
	templates   *template.Template
	itemCatalog map[string]catalogEntry
}

func NewServer(store *SessionStore, templates *template.Template, itemCatalog map[string]catalogEntry) *Server {
	return &Server{store: store, templates: templates, itemCatalog: itemCatalog}
}
```

Add the route in `routes()`:

```go
mux.HandleFunc("GET /session/{id}/inventory", s.handleInventory)
```

Add the handler:

```go
func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}
	sess.mu.RLock()
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	sess.mu.RUnlock()
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "inventory", view); err != nil {
		log.Printf("rendering inventory: %v", err)
	}
}
```

- [ ] **Step 4: Update `main.go` to load the catalog at startup**

```go
tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
if err != nil {
	log.Fatalf("parsing templates: %v", err)
}

itemsData, err := embeddedStaticFS.ReadFile("static/items.json")
if err != nil {
	log.Fatalf("reading items.json: %v", err)
}
catalog, err := loadItemCatalog(itemsData)
if err != nil {
	log.Fatalf("parsing items.json: %v", err)
}

srv := NewServer(NewSessionStore(), tmpl, catalog)
```

- [ ] **Step 5: Update `newTestServer` in `handlers_test.go`**

```go
func newTestServer(t *testing.T) *Server {
	t.Helper()
	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		t.Fatalf("parsing templates: %v", err)
	}
	sub, err := fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		t.Fatalf("static assets: %v", err)
	}
	staticFS = sub
	itemsData, err := embeddedStaticFS.ReadFile("static/items.json")
	if err != nil {
		t.Fatalf("reading items.json: %v", err)
	}
	catalog, err := loadItemCatalog(itemsData)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	return NewServer(NewSessionStore(), tmpl, catalog)
}
```

- [ ] **Step 6: Write `templates/inventory.html.tmpl`**

```html
{{define "inventory"}}
<div class="inventory-grid">
  <section class="backpack">
    <h3>Backpack</h3>
    <div class="slot-grid">
      {{range .Backpack}}{{template "slot" .}}{{end}}
    </div>
  </section>
  <section class="hotbar">
    <h3>Hotbar</h3>
    <div class="slot-grid hotbar-row">
      {{range .Hotbar}}{{template "slot" .}}{{end}}
    </div>
  </section>
  <section class="equipment">
    <h3>Equipment</h3>
    <div class="equipment-slots">
      {{range .Equipment}}{{template "slot" .}}{{end}}
    </div>
  </section>
</div>
{{end}}

{{define "slot"}}
<div class="slot{{if not .Occupied}} slot-empty{{end}}" id="slot-{{.RowID}}">
  {{if .Label}}<span class="slot-label">{{.Label}}</span>{{end}}
  {{if .Occupied}}
    <img class="slot-icon" src="/static/icons/{{if .IconFile}}{{.IconFile}}{{else}}_placeholder.png{{end}}" alt="{{.Name}}" title="{{.Name}}">
    {{if gt .Quantity 1}}<span class="slot-qty">{{.Quantity}}</span>{{end}}
  {{end}}
  {{if .Error}}<span class="error">{{.Error}}</span>{{end}}
</div>
{{end}}
{{end}}
```

- [ ] **Step 7: Add the tab bar to `templates/index.html.tmpl`**

Replace the body's upload/tree area:

```html
<form id="upload-form" hx-post="/upload" hx-target="#app" hx-swap="innerHTML" hx-encoding="multipart/form-data">
  <div id="upload-zone" class="upload-zone">
    <p>Drop a <code>.sav</code> file here, or <label for="savefile" class="browse-link">browse…</label></p>
    <input type="file" id="savefile" name="savefile" accept=".sav" required>
  </div>
  <button type="submit">Upload</button>
</form>
<div id="download-area"></div>
<div id="app"></div>
```

And in `templates/children.html.tmpl`'s `uploadResponse` define, replace:

```html
{{define "uploadResponse"}}
<div id="download-area" hx-swap-oob="true"><a href="/session/{{.SessionID}}/download">Download edited save</a></div>
{{template "children" .}}
{{end}}
```

with:

```html
{{define "uploadResponse"}}
<div id="download-area" hx-swap-oob="true"><a href="/session/{{.SessionID}}/download">Download edited save</a></div>
<div class="tabs">
  <button type="button" class="tab-btn" hx-get="/session/{{.SessionID}}/children?path=" hx-target="#tab-content" hx-swap="innerHTML">Tree</button>
  <button type="button" class="tab-btn" hx-get="/session/{{.SessionID}}/inventory" hx-target="#tab-content" hx-swap="innerHTML">Inventory</button>
</div>
<div id="tab-content">
  {{template "children" .}}
</div>
{{end}}
{{end}}
```

- [ ] **Step 8: Add grid/tab CSS to `static/style.css`**

```css
.tabs {
  display: flex;
  gap: 0.5rem;
  margin: 1rem 0 0.5rem;
}
.tab-btn {
  padding: 0.4rem 1rem;
  border: 1px solid #ccc;
  border-radius: 6px 6px 0 0;
  background: #f5f5f5;
  cursor: pointer;
  font-size: 0.95rem;
}
.tab-btn:hover {
  background: #eee;
}

.inventory-grid section {
  margin-bottom: 1.5rem;
}
.inventory-grid h3 {
  margin: 0 0 0.5rem;
  font-size: 1rem;
  color: #555;
}
.slot-grid {
  display: grid;
  grid-template-columns: repeat(9, 3rem);
  gap: 0.25rem;
}
.equipment-slots {
  display: grid;
  grid-template-columns: repeat(4, 3rem);
  gap: 0.25rem;
  max-width: 14rem;
}
.slot {
  position: relative;
  width: 3rem;
  height: 3rem;
  border: 1px solid #bbb;
  border-radius: 4px;
  background: #fafafa;
  display: flex;
  align-items: center;
  justify-content: center;
}
.slot-empty {
  background: #f0f0f0;
}
.slot-icon {
  width: 2.5rem;
  height: 2.5rem;
  image-rendering: pixelated;
}
.slot-qty {
  position: absolute;
  bottom: 1px;
  right: 3px;
  font-size: 0.7rem;
  color: #fff;
  text-shadow: 0 0 2px #000, 0 0 2px #000;
}
.slot-label {
  position: absolute;
  top: -1.1rem;
  left: 0;
  font-size: 0.65rem;
  color: #888;
  white-space: nowrap;
}
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./... -v -run 'TestHandleInventory|TestHandleIndex|TestHandleUpload'`
Expected: PASS.

- [ ] **Step 10: Run the full test suite to confirm no regressions**

Run: `go test ./... -race`
Expected: PASS.

- [ ] **Step 11: Manual smoke test**

```bash
go run . -addr :18099 &
sleep 1
curl -s -F "savefile=@testdata/Player_Local.sav" http://localhost:18099/upload | head -c 300
kill %1
```

Then open a browser to `http://localhost:18099`, upload `testdata/Player_Local.sav`, click the "Inventory" tab, and visually confirm: a 9-wide backpack grid, a separate hotbar row, an equipment panel, real item icons (not all placeholders), and quantity badges on stacked items.

- [ ] **Step 12: Commit**

```bash
git add main.go handlers.go handlers_test.go handlers_inventory_test.go templates/inventory.html.tmpl templates/index.html.tmpl templates/children.html.tmpl static/style.css
git commit -m "feat: add view-only Inventory tab (grid + equipment panel)"
```

---

### Task 7: Click-to-edit item swap and quantity (occupied slots)

**Files:**
- Modify: `handlers.go` (`handleEdit` gains slot-aware rendering; new `renderSlot` helper)
- Modify: `templates/inventory.html.tmpl` (`slot` define gains edit forms for occupied slots)
- Modify: `handlers_inventory_test.go`

**Interfaces:**
- Consumes: `buildSlotView`, `componentSlotsPath` (Task 3); `applyEdit`, `previewOf` (existing, `handlers.go`).
- Produces: `func (s *Server) renderSlot(w http.ResponseWriter, f *gvas.File, sessionID, slotPath string, status int, errMsg string)`, reused by Task 8's add/remove handlers.

- [ ] **Step 1: Write the failing test**

```go
func TestHandleEditFromInventoryTabRerendersSlot(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	sess, _ := s.store.Get(id)
	slotPath := "" // filled in below once we know the real path
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath = view.Backpack[0].SlotPath
	itemPath := view.Backpack[0].ItemPath

	form := url.Values{}
	form.Set("path", itemPath)
	form.Set("value", "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit_EDITED")
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleEdit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="slot`) {
		t.Errorf("expected a re-rendered <div class=\"slot...\">, got a leafRow instead:\n%s", body)
	}
	if strings.Contains(body, `<li class="leaf"`) {
		t.Error("got a leafRow fragment; slotPath should have routed this to the slot template instead")
	}
}

func TestHandleEditFromTreeTabStillRendersLeafRow(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	form := url.Values{}
	form.Set("path", "IslandID")
	form.Set("value", "changed-value")
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleEdit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<li class="leaf"`) {
		t.Errorf("expected the existing leafRow fragment (no slotPath was sent), got:\n%s", rec.Body.String())
	}
}
```

This task's test file needs `net/url` imported alongside the existing `strings`/`net/http/httptest` imports already present in `handlers_inventory_test.go` from Task 6.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestHandleEditFromInventoryTab|TestHandleEditFromTreeTab' -v`
Expected: `TestHandleEditFromInventoryTabRerendersSlot` FAILs (still gets a `leafRow`); `TestHandleEditFromTreeTabStillRendersLeafRow` PASSes already (no behavior change yet for that path).

- [ ] **Step 3: Extend `handleEdit` and add `renderSlot`**

In `handlers.go`, change `handleEdit` from:

```go
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	path := r.FormValue("path")
	value := r.FormValue("value")

	sess.mu.Lock()
	defer sess.mu.Unlock()

	p, err := gvas.Lookup(sess.File, path)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		s.templates.ExecuteTemplate(w, "leafRow", itemView{
			childItem: childItem{Label: lastPathSegment(path), Path: path, Editable: true, Preview: value},
			SessionID: id,
			RowID:     rowID(path),
			Error:     err.Error(),
		})
		return
	}

	preview, editErr := applyEdit(p, value)
	status := http.StatusOK
	errMsg := ""
	if editErr != nil {
		status = http.StatusBadRequest
		errMsg = editErr.Error()
		preview = previewOf(p)
	}

	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "leafRow", itemView{
		childItem: childItem{Label: lastPathSegment(path), Type: p.Type, Path: path, Editable: true, Preview: preview},
		SessionID: id,
		RowID:     rowID(path),
		Error:     errMsg,
	})
}
```

to:

```go
func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	path := r.FormValue("path")
	value := r.FormValue("value")
	slotPath := r.FormValue("slotPath") // set only by Inventory-tab edit forms; "" for Tree-tab edits

	sess.mu.Lock()
	defer sess.mu.Unlock()

	p, err := gvas.Lookup(sess.File, path)
	if err != nil {
		if slotPath != "" {
			s.renderSlot(w, sess.File, id, slotPath, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		s.templates.ExecuteTemplate(w, "leafRow", itemView{
			childItem: childItem{Label: lastPathSegment(path), Path: path, Editable: true, Preview: value},
			SessionID: id,
			RowID:     rowID(path),
			Error:     err.Error(),
		})
		return
	}

	preview, editErr := applyEdit(p, value)
	status := http.StatusOK
	errMsg := ""
	if editErr != nil {
		status = http.StatusBadRequest
		errMsg = editErr.Error()
		preview = previewOf(p)
	}

	if slotPath != "" {
		s.renderSlot(w, sess.File, id, slotPath, status, errMsg)
		return
	}

	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "leafRow", itemView{
		childItem: childItem{Label: lastPathSegment(path), Type: p.Type, Path: path, Editable: true, Preview: preview},
		SessionID: id,
		RowID:     rowID(path),
		Error:     errMsg,
	})
}

// renderSlot re-renders one Inventory-tab slot from f's current state,
// used after an edit, add, or remove targeting that slot. status/errMsg
// let a failed operation still re-render the slot (with its unchanged
// data) alongside an inline error, matching the Tree tab's leafRow
// error-handling pattern.
func (s *Server) renderSlot(w http.ResponseWriter, f *gvas.File, sessionID, slotPath string, status int, errMsg string) {
	slotProp, err := gvas.Lookup(f, slotPath)
	if err != nil || slotProp.Struct == nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("re-rendering slot %q: %w", slotPath, err))
		return
	}
	sv := buildSlotView(slotProp.Struct, slotPath, sessionID, s.itemCatalog)
	sv.Error = errMsg
	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "slot", sv)
}
```

- [ ] **Step 4: Add edit forms to the occupied-slot template**

In `templates/inventory.html.tmpl`, replace the `slot` define's occupied branch:

```html
{{define "slot"}}
<div class="slot{{if not .Occupied}} slot-empty{{end}}" id="slot-{{.RowID}}">
  {{if .Label}}<span class="slot-label">{{.Label}}</span>{{end}}
  {{if .Occupied}}
    <form class="slot-item-form" hx-post="/session/{{.SessionID}}/edit" hx-target="#slot-{{.RowID}}" hx-swap="outerHTML">
      <input type="hidden" name="path" value="{{.ItemPath}}">
      <input type="hidden" name="slotPath" value="{{.SlotPath}}">
      <span class="item-picker">
        <img class="slot-icon" src="/static/icons/{{if .IconFile}}{{.IconFile}}{{else}}_placeholder.png{{end}}" alt="{{.Name}}" title="{{.Name}}">
        <input type="text" name="value" value="{{.ObjectPath}}" data-item-picker="1" autocomplete="off" class="slot-item-input">
      </span>
    </form>
    <form class="slot-qty-form" hx-post="/session/{{.SessionID}}/edit" hx-target="#slot-{{.RowID}}" hx-swap="outerHTML">
      <input type="hidden" name="path" value="{{.QtyPath}}">
      <input type="hidden" name="slotPath" value="{{.SlotPath}}">
      <input type="number" name="value" value="{{.Quantity}}" min="0" class="slot-qty-input">
    </form>
    {{if gt .Quantity 1}}<span class="slot-qty">{{.Quantity}}</span>{{end}}
  {{end}}
  {{if .Error}}<span class="error">{{.Error}}</span>{{end}}
</div>
{{end}}
{{end}}
```

(This removes the old bare `<span class="slot-qty">` duplicate placement from Task 6's version — it now lives after the quantity form so the badge still renders visually, but the editable number input is the actual control.)

- [ ] **Step 5: Add minimal CSS for the new forms**

Append to `static/style.css`:

```css
.slot-item-form, .slot-qty-form {
  display: contents;
}
.slot-item-input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}
.slot-qty-input {
  position: absolute;
  bottom: 0;
  right: 0;
  width: 1.8rem;
  font-size: 0.65rem;
  border: none;
  background: transparent;
  text-align: right;
  color: #fff;
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./... -run 'TestHandleEditFromInventoryTab|TestHandleEditFromTreeTab' -v`
Expected: PASS.

- [ ] **Step 7: Run the full test suite to confirm no regressions**

Run: `go test ./... -race`
Expected: PASS.

- [ ] **Step 8: Manual smoke test**

Repeat Task 6 Step 11's server start; on the Inventory tab, click an occupied slot's icon (item picker should open), pick a different item, confirm the icon updates in place; edit a quantity number and confirm it saves.

- [ ] **Step 9: Commit**

```bash
git add handlers.go templates/inventory.html.tmpl static/style.css handlers_inventory_test.go
git commit -m "feat: click-to-edit item swap and quantity on the Inventory tab"
```

---

### Task 8: Add/remove items (empty and occupied slots)

**Files:**
- Create: `itemtemplate.go`
- Create: `itemtemplate_test.go`
- Modify: `handlers.go` (`handleSlotAdd`, `handleSlotRemove`, two new routes)
- Modify: `templates/inventory.html.tmpl` (empty-slot add form; occupied-slot remove button)
- Modify: `handlers_inventory_test.go`

**Interfaces:**
- Consumes: `AppendStructElement`/`RemoveStructElement` (Task 1, via the `skyversesave` module's local `replace` directive -- already available, no `go.mod` change needed), `componentSlotsPath`/`findFieldProp` (Task 3), `s.renderSlot` (Task 7).

- [ ] **Step 1: Write the failing tests for `itemtemplate.go`**

```go
// itemtemplate_test.go
package main

import (
	"testing"

	"skyversesave/gvas"
)

func TestBuildNewItemClonesExistingMatch(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	existingPath := "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"

	item, err := buildNewItem(f, existingPath)
	if err != nil {
		t.Fatalf("buildNewItem: %v", err)
	}
	base := findFieldProp(item, "BaseData")
	if base == nil || base.Str == nil || *base.Str != existingPath {
		t.Fatalf("BaseData = %+v, want %q", base, existingPath)
	}
	qty := findFieldProp(item, "SpecialEffectLevel")
	if qty == nil || qty.Int32 == nil || *qty.Int32 != 1 {
		t.Errorf("cloned item's SpecialEffectLevel = %+v, want 1 (matching the real RepairKit slot's value)", qty)
	}
	uid := findFieldProp(item, "UniqueID")
	if uid == nil || uid.Str == nil || *uid.Str != "" {
		t.Errorf("cloned item's UniqueID = %+v, want empty (always reset)", uid)
	}
}

func TestBuildNewItemFallsBackToGenericTemplate(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	newPath := "/Game/Data/Items/SomeCategory/IDA_NeverSeenBefore.IDA_NeverSeenBefore"

	item, err := buildNewItem(f, newPath)
	if err != nil {
		t.Fatalf("buildNewItem: %v", err)
	}
	base := findFieldProp(item, "BaseData")
	if base == nil || base.Str == nil || *base.Str != newPath {
		t.Fatalf("BaseData = %+v, want %q", base, newPath)
	}
	level := findFieldProp(item, "Level")
	if level == nil || level.Int32 == nil || *level.Int32 != 0 {
		t.Errorf("Level = %+v, want 0", level)
	}
	durability := findFieldProp(item, "bHasDurability")
	if durability == nil || durability.Bool == nil || *durability.Bool != false {
		t.Errorf("bHasDurability = %+v, want false", durability)
	}
}

func TestDeepCopyPropsDoesNotAliasOriginal(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	original := findFieldProp(f.Root, "IslandID")
	copied := deepCopyProps([]*gvas.Property{original})
	newVal := "mutated-copy-only"
	copied[0].Str = &newVal

	if *original.Str == newVal {
		t.Fatal("mutating the copy also mutated the original -- deepCopyProps aliased a pointer")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestBuildNewItem|TestDeepCopyProps' -v`
Expected: FAIL with `undefined: buildNewItem` / `undefined: deepCopyProps`.

- [ ] **Step 3: Implement `itemtemplate.go`**

```go
// itemtemplate.go
package main

import (
	"fmt"

	"skyversesave/gvas"
)

// buildNewItem returns the property list for a new item referencing
// objectPath, ready to append into an empty slot's Items array. If an
// item with this exact BaseData already exists elsewhere in the save
// (Inventory or Equipment), its full property list is cloned -- correct
// durability, stat rolls, fuel state, etc. -- with only UniqueID reset.
// Otherwise, a plain (non-durability) existing item elsewhere in the
// save is cloned as a *structural* template (so this format's nested
// struct-array metadata, e.g. CustomModificators' inner struct name,
// stays byte-correct without needing to be hand-authored) and its
// scalar fields are overwritten with the plain-item defaults every
// stackable resource/consumable in a real save already has. See
// docs/superpowers/specs/2026-09-19-inventory-tab-design.md,
// "Best-effort item construction".
func buildNewItem(f *gvas.File, objectPath string) ([]*gvas.Property, error) {
	if exact := findExistingItem(f, objectPath); exact != nil {
		return exact, nil
	}
	template := findPlainItemTemplate(f)
	if template == nil {
		return nil, fmt.Errorf("no existing plain item found in this save to use as an add-item template")
	}
	item := deepCopyProps(template)
	setFieldString(item, "BaseData", objectPath)
	setFieldInt32(item, "Level", 0)
	setFieldInt32(item, "Durability", 1)
	setFieldBool(item, "bHasDurability", false)
	setFieldInt32(item, "Energy", -1)
	setFieldString(item, "FuelType", "")
	setFieldFloat32(item, "Fuel", -1)
	setFieldString(item, "SpecialEffect", "")
	setFieldInt32(item, "SpecialEffectLevel", 1)
	setFieldString(item, "UniqueID", "")
	clearFieldArray(item, "CustomModificators")
	clearFieldArray(item, "AdditionalAlchemyEffectsList")
	clearFieldArray(item, "AdditionalAlchemyEffectsTiers")
	return item, nil
}

// findExistingItem searches every occupied slot in both the Inventory
// and Equipment components for an Items[0] whose BaseData equals
// objectPath, returning a deep copy of its full property list (with
// UniqueID reset to ""), or nil if no match exists anywhere in the save.
func findExistingItem(f *gvas.File, objectPath string) []*gvas.Property {
	item := findFirstItemMatching(f, func(itemProps []*gvas.Property) bool {
		base := findFieldProp(itemProps, "BaseData")
		return base != nil && base.Str != nil && *base.Str == objectPath
	})
	if item == nil {
		return nil
	}
	copied := deepCopyProps(item)
	setFieldString(copied, "UniqueID", "")
	return copied
}

// findPlainItemTemplate returns (without copying) the first occupied
// slot's item whose bHasDurability is false -- a "plain
// resource/consumable" shape, used as the structural template for a
// brand-new item when no exact BaseData match exists anywhere in the
// save.
func findPlainItemTemplate(f *gvas.File) []*gvas.Property {
	return findFirstItemMatching(f, func(itemProps []*gvas.Property) bool {
		d := findFieldProp(itemProps, "bHasDurability")
		return d != nil && d.Bool != nil && !*d.Bool
	})
}

// findFirstItemMatching walks every Slots[i].Items[0] in the Inventory
// and Equipment components and returns the first one for which match
// returns true, or nil.
func findFirstItemMatching(f *gvas.File, match func(itemProps []*gvas.Property) bool) []*gvas.Property {
	for _, componentName := range []string{"Inventory", "Equipment"} {
		slotsPath, err := componentSlotsPath(f, componentName)
		if err != nil {
			continue
		}
		slots, err := gvas.Lookup(f, slotsPath)
		if err != nil || slots.Array == nil {
			continue
		}
		for _, slotProps := range slots.Array.Structs {
			items := findFieldProp(slotProps, "Items")
			if items == nil || items.Array == nil || len(items.Array.Structs) == 0 {
				continue
			}
			itemProps := items.Array.Structs[0]
			if match(itemProps) {
				return itemProps
			}
		}
	}
	return nil
}

func setFieldString(props []*gvas.Property, name, value string) {
	if p := findFieldProp(props, name); p != nil {
		p.SetString(value)
	}
}
func setFieldInt32(props []*gvas.Property, name string, value int32) {
	if p := findFieldProp(props, name); p != nil {
		p.SetInt32(value)
	}
}
func setFieldFloat32(props []*gvas.Property, name string, value float32) {
	if p := findFieldProp(props, name); p != nil {
		p.SetFloat32(value)
	}
}
func setFieldBool(props []*gvas.Property, name string, value bool) {
	if p := findFieldProp(props, name); p != nil {
		p.SetBool(value)
	}
}
func clearFieldArray(props []*gvas.Property, name string) {
	if p := findFieldProp(props, name); p != nil && p.Array != nil {
		p.Array.Structs = nil
	}
}

// deepCopyProps recursively copies a property list so mutating the copy
// never aliases the original slot's data.
func deepCopyProps(props []*gvas.Property) []*gvas.Property {
	out := make([]*gvas.Property, len(props))
	for i, p := range props {
		cp := *p
		if p.Bool != nil {
			v := *p.Bool
			cp.Bool = &v
		}
		if p.Str != nil {
			v := *p.Str
			cp.Str = &v
		}
		if p.Int32 != nil {
			v := *p.Int32
			cp.Int32 = &v
		}
		if p.Int64 != nil {
			v := *p.Int64
			cp.Int64 = &v
		}
		if p.Float32 != nil {
			v := *p.Float32
			cp.Float32 = &v
		}
		if p.Float64 != nil {
			v := *p.Float64
			cp.Float64 = &v
		}
		if p.Byte != nil {
			v := *p.Byte
			cp.Byte = &v
		}
		if p.Guid != nil {
			cp.Guid = append([]byte(nil), p.Guid...)
		}
		if p.Raw != nil {
			cp.Raw = append([]byte(nil), p.Raw...)
		}
		if p.Struct != nil {
			cp.Struct = deepCopyProps(p.Struct)
		}
		if p.Array != nil {
			cp.Array = deepCopyArrayValue(p.Array)
		}
		out[i] = &cp
	}
	return out
}

func deepCopyArrayValue(a *gvas.ArrayValue) *gvas.ArrayValue {
	cp := *a
	if a.Structs != nil {
		cp.Structs = make([][]*gvas.Property, len(a.Structs))
		for i, elem := range a.Structs {
			cp.Structs[i] = deepCopyProps(elem)
		}
	}
	if a.Bools != nil {
		cp.Bools = append([]bool(nil), a.Bools...)
	}
	if a.Ints != nil {
		cp.Ints = append([]int32(nil), a.Ints...)
	}
	if a.Int64s != nil {
		cp.Int64s = append([]int64(nil), a.Int64s...)
	}
	if a.Floats != nil {
		cp.Floats = append([]float32(nil), a.Floats...)
	}
	if a.Doubles != nil {
		cp.Doubles = append([]float64(nil), a.Doubles...)
	}
	if a.Strings != nil {
		cp.Strings = append([]string(nil), a.Strings...)
	}
	if a.Bytes != nil {
		cp.Bytes = append([]byte(nil), a.Bytes...)
	}
	if a.RawElements != nil {
		cp.RawElements = append([]byte(nil), a.RawElements...)
	}
	return &cp
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestBuildNewItem|TestDeepCopyProps' -v`
Expected: PASS.

- [ ] **Step 5: Write the failing handler tests**

```go
// append to handlers_inventory_test.go
func TestHandleSlotRemove(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Backpack[0].SlotPath // occupied (RepairKit) in testdata

	form := url.Values{}
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotRemove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "/static/icons/") {
		t.Errorf("expected the slot to render empty after removal, got:\n%s", rec.Body.String())
	}

	view2, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView after remove: %v", err)
	}
	if view2.Backpack[0].Occupied {
		t.Error("Backpack[0] still Occupied=true after remove")
	}
}

func TestHandleSlotRemoveAlreadyEmpty(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Hotbar[8].SlotPath // empty in testdata

	form := url.Values{}
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotRemove(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (slot already empty)", rec.Code)
	}
}

func TestHandleSlotAdd(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Hotbar[8].SlotPath // empty in testdata
	itemPath := "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", itemPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotAdd(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	view2, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView after add: %v", err)
	}
	if !view2.Hotbar[8].Occupied {
		t.Fatal("Hotbar[8] still Occupied=false after add")
	}
	if view2.Hotbar[8].ObjectPath != itemPath {
		t.Errorf("Hotbar[8].ObjectPath = %q, want %q", view2.Hotbar[8].ObjectPath, itemPath)
	}
}

func TestHandleSlotAddUnknownItem(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Hotbar[8].SlotPath

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", "/Game/Not/A/Real/Item.Item")
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotAdd(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unknown item)", rec.Code)
	}
}

func TestHandleSlotAddAlreadyOccupied(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Backpack[0].SlotPath // occupied

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit")
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotAdd(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (slot already occupied)", rec.Code)
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./... -run 'TestHandleSlot' -v`
Expected: FAIL to compile (`s.handleSlotRemove`/`s.handleSlotAdd` undefined).

- [ ] **Step 7: Implement the handlers and routes**

In `handlers.go`, add to `routes()`:

```go
mux.HandleFunc("POST /session/{id}/slot/add", s.handleSlotAdd)
mux.HandleFunc("POST /session/{id}/slot/remove", s.handleSlotRemove)
```

Add the handlers:

```go
func (s *Server) handleSlotRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	slotPath := r.FormValue("slotPath")

	sess.mu.Lock()
	defer sess.mu.Unlock()

	itemsProp, err := gvas.Lookup(sess.File, slotPath+".Items")
	if err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("resolving slot: %w", err))
		return
	}
	if itemsProp.Array == nil || len(itemsProp.Array.Structs) == 0 {
		renderError(w, http.StatusBadRequest, fmt.Errorf("slot is already empty"))
		return
	}
	if err := itemsProp.RemoveStructElement(0); err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.renderSlot(w, sess.File, id, slotPath, http.StatusOK, "")
}

func (s *Server) handleSlotAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	slotPath := r.FormValue("slotPath")
	itemPath := r.FormValue("itemPath")

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if _, ok := s.itemCatalog[itemPath]; !ok {
		renderError(w, http.StatusBadRequest, fmt.Errorf("unknown item %q", itemPath))
		return
	}
	itemsProp, err := gvas.Lookup(sess.File, slotPath+".Items")
	if err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("resolving slot: %w", err))
		return
	}
	if itemsProp.Array == nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("slot %q has no Items array", slotPath))
		return
	}
	if len(itemsProp.Array.Structs) != 0 {
		renderError(w, http.StatusBadRequest, fmt.Errorf("slot already has an item"))
		return
	}
	newItem, err := buildNewItem(sess.File, itemPath)
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
	if err := itemsProp.AppendStructElement(newItem); err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
	s.renderSlot(w, sess.File, id, slotPath, http.StatusOK, "")
}
```

- [ ] **Step 8: Wire the empty-slot add form and occupied-slot remove button**

In `templates/inventory.html.tmpl`, replace the `slot` define's `{{if .Occupied}}...{{end}}` block with:

```html
{{if .Occupied}}
    <form class="slot-item-form" hx-post="/session/{{.SessionID}}/edit" hx-target="#slot-{{.RowID}}" hx-swap="outerHTML">
      <input type="hidden" name="path" value="{{.ItemPath}}">
      <input type="hidden" name="slotPath" value="{{.SlotPath}}">
      <span class="item-picker">
        <img class="slot-icon" src="/static/icons/{{if .IconFile}}{{.IconFile}}{{else}}_placeholder.png{{end}}" alt="{{.Name}}" title="{{.Name}}">
        <input type="text" name="value" value="{{.ObjectPath}}" data-item-picker="1" autocomplete="off" class="slot-item-input">
      </span>
    </form>
    <form class="slot-qty-form" hx-post="/session/{{.SessionID}}/edit" hx-target="#slot-{{.RowID}}" hx-swap="outerHTML">
      <input type="hidden" name="path" value="{{.QtyPath}}">
      <input type="hidden" name="slotPath" value="{{.SlotPath}}">
      <input type="number" name="value" value="{{.Quantity}}" min="0" class="slot-qty-input">
    </form>
    {{if gt .Quantity 1}}<span class="slot-qty">{{.Quantity}}</span>{{end}}
    <form class="slot-remove-form" hx-post="/session/{{.SessionID}}/slot/remove" hx-target="#slot-{{.RowID}}" hx-swap="outerHTML">
      <input type="hidden" name="slotPath" value="{{.SlotPath}}">
      <button type="submit" class="slot-remove-btn" title="Remove item">×</button>
    </form>
  {{else}}
    <form class="slot-add-form" hx-post="/session/{{.SessionID}}/slot/add" hx-target="#slot-{{.RowID}}" hx-swap="outerHTML">
      <input type="hidden" name="slotPath" value="{{.SlotPath}}">
      <span class="item-picker">
        <input type="text" name="itemPath" placeholder="+" data-item-picker="1" autocomplete="off" class="slot-add-input">
      </span>
    </form>
  {{end}}
```

- [ ] **Step 9: CSS for the remove button and add input**

Append to `static/style.css`:

```css
.slot-remove-btn {
  position: absolute;
  top: -2px;
  right: -2px;
  width: 1.1rem;
  height: 1.1rem;
  line-height: 1;
  border: none;
  border-radius: 50%;
  background: #c33;
  color: #fff;
  font-size: 0.7rem;
  cursor: pointer;
}
.slot-add-input {
  width: 100%;
  height: 100%;
  border: none;
  background: transparent;
  text-align: center;
  font-size: 1.2rem;
  color: #999;
  cursor: pointer;
}
```

- [ ] **Step 10: Run tests to verify they pass**

Run: `go test ./... -run 'TestHandleSlot' -v`
Expected: PASS.

- [ ] **Step 11: Run the full test suite to confirm no regressions**

Run: `go test ./... -race`
Expected: PASS.

- [ ] **Step 12: Manual smoke test**

Repeat the server-start smoke test; on the Inventory tab, click the "×" on an occupied slot and confirm it empties; click an empty slot's "+" and add an item via the picker, confirm the icon appears; download the edited save and re-upload it, confirming the changes persisted.

- [ ] **Step 13: Commit**

```bash
git add itemtemplate.go itemtemplate_test.go handlers.go templates/inventory.html.tmpl static/style.css handlers_inventory_test.go
git commit -m "feat: add/remove items on the Inventory tab (best-effort item construction)"
```

---

## Self-Review Notes

- **Spec coverage:** every section of `2026-09-19-inventory-tab-design.md` maps to a task -- data mapping (Task 3), icon pipeline (Tasks 4-5), gvas primitives (Task 1), best-effort construction (Task 8), new components/endpoints (Tasks 6-8), tab switching (Task 6), error handling (woven through Tasks 6-8's handlers), testing (every task).
- **Type consistency:** `slotView`'s full field set was finalized once (Task 3) with every field a later task needs (`SlotPath`, `RowID`, `ObjectPath`, `Error`) declared upfront, rather than retrofitted -- Tasks 6-8 only ever consume fields already in Task 3's definition. Task 5's `main.go` was fixed during self-review to use `iconextract.Decode`'s real return type (`*image.NRGBA`) throughout, including its own `newPlaceholder` helper, rather than an invented `iconextract.NRGBAImage` type.
- **Placeholder scan:** no "TBD"/"handle edge cases"-style placeholders remain; every step has concrete code or an exact command.
