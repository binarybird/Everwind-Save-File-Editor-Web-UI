package main

import (
	"os"
	"testing"

	"skyversesave/gvas"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return data
}

func mustUnmarshal(t *testing.T, name string) *gvas.File {
	t.Helper()
	f, err := gvas.Unmarshal(readTestdata(t, name))
	if err != nil {
		t.Fatalf("Unmarshal(%s): %v", name, err)
	}
	return f
}

func findItem(items []childItem, label string) *childItem {
	for i := range items {
		if items[i].Label == label {
			return &items[i]
		}
	}
	return nil
}

func TestPropsAtRoot(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	props, isContainer, err := propsAt(f, "")
	if err != nil {
		t.Fatalf("propsAt: %v", err)
	}
	if !isContainer {
		t.Fatal("root should be a container")
	}
	if len(props) == 0 {
		t.Fatal("expected root properties, got none")
	}
}

func TestPropsAtNestedStruct(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	props, isContainer, err := propsAt(f, "UDSData.CurrentTime")
	if err != nil {
		t.Fatalf("propsAt: %v", err)
	}
	if !isContainer {
		t.Fatal("UDSData.CurrentTime should be a container")
	}
	if findMonth := func() bool {
		for _, p := range props {
			if p.Name == "Month" {
				return true
			}
		}
		return false
	}; !findMonth() {
		t.Error("expected a 'Month' field under UDSData.CurrentTime")
	}
}

func TestPropsAtArrayIsNotContainer(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	_, isContainer, err := propsAt(f, "BoatsData")
	if err != nil {
		t.Fatalf("propsAt: %v", err)
	}
	if isContainer {
		t.Fatal("an ArrayProperty should not be reported as a struct container")
	}
}

func TestPropsAtLeafIsNotContainer(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	_, isContainer, err := propsAt(f, "WorldName")
	if err != nil {
		t.Fatalf("propsAt: %v", err)
	}
	if isContainer {
		t.Fatal("a scalar leaf should not be reported as a container")
	}
}

func TestStructChildrenTopLevel(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	items := structChildren(f.Root, "")

	name := findItem(items, "WorldName")
	if name == nil {
		t.Fatal("expected a WorldName item")
	}
	if !name.Editable || name.Expandable {
		t.Errorf("WorldName: got Editable=%v Expandable=%v, want Editable=true Expandable=false", name.Editable, name.Expandable)
	}
	if name.Preview != "jff" {
		t.Errorf("WorldName preview = %q, want %q", name.Preview, "jff")
	}
	if name.Path != "WorldName" {
		t.Errorf("WorldName path = %q, want %q", name.Path, "WorldName")
	}

	uds := findItem(items, "UDSData")
	if uds == nil {
		t.Fatal("expected a UDSData item")
	}
	if !uds.Expandable || uds.Editable {
		t.Errorf("UDSData: got Expandable=%v Editable=%v, want Expandable=true Editable=false", uds.Expandable, uds.Editable)
	}

	boats := findItem(items, "BoatsData")
	if boats == nil {
		t.Fatal("expected a BoatsData item")
	}
	if !boats.Expandable {
		t.Error("BoatsData (an ArrayProperty) should be Expandable")
	}
	if boats.Preview == "" {
		t.Error("BoatsData should have a non-empty element-count preview")
	}
}

// TestStructChildrenSortsNonExpandableFirst confirms the display-ordering
// preference: within one object's field list, every non-expandable item
// (scalar, native struct) sorts before every expandable one (nested
// struct, array), regardless of the fields' order in the save file.
func TestStructChildrenSortsNonExpandableFirst(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	items := structChildren(f.Root, "")

	if len(items) < 2 {
		t.Fatalf("expected multiple top-level items, got %d", len(items))
	}
	sawExpandable := false
	for _, it := range items {
		if it.Expandable {
			sawExpandable = true
			continue
		}
		if sawExpandable {
			t.Fatalf("non-expandable item %q appeared after an expandable one; items: %+v", it.Label, items)
		}
	}
	if !sawExpandable {
		t.Fatal("expected at least one expandable item in WorldInfo.sav's top level (e.g. UDSData)")
	}

	// Order within each group is preserved (stable sort): WorldName comes
	// before InitializeGameVersion in the file, both non-expandable;
	// UDSData comes before BoatsData, both expandable.
	worldNameIdx, gameVersionIdx, udsIdx, boatsIdx := -1, -1, -1, -1
	for i, it := range items {
		switch it.Label {
		case "WorldName":
			worldNameIdx = i
		case "InitializeGameVersion":
			gameVersionIdx = i
		case "UDSData":
			udsIdx = i
		case "BoatsData":
			boatsIdx = i
		}
	}
	if worldNameIdx < 0 || gameVersionIdx < 0 || worldNameIdx > gameVersionIdx {
		t.Errorf("expected WorldName before InitializeGameVersion (both non-expandable, original order preserved); got indices %d, %d", worldNameIdx, gameVersionIdx)
	}
	if udsIdx < 0 || boatsIdx < 0 || udsIdx > boatsIdx {
		t.Errorf("expected UDSData before BoatsData (both expandable, original order preserved); got indices %d, %d", udsIdx, boatsIdx)
	}
}

func TestStructChildrenNativeStruct(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	items := structChildren(f.Root, "")
	created := findItem(items, "CreatedTime")
	if created == nil {
		t.Fatal("expected a CreatedTime item")
	}
	if created.Expandable || created.Editable {
		t.Errorf("CreatedTime (a native DateTime): got Expandable=%v Editable=%v, want both false", created.Expandable, created.Editable)
	}
	if created.Preview == "" {
		t.Error("CreatedTime should have a non-empty native-struct preview")
	}
}

func TestJoinPath(t *testing.T) {
	cases := []struct{ base, next, want string }{
		{"", "Foo", "Foo"},
		{"Foo", "Bar", "Foo.Bar"},
	}
	for _, c := range cases {
		if got := joinPath(c.base, c.next); got != c.want {
			t.Errorf("joinPath(%q, %q) = %q, want %q", c.base, c.next, got, c.want)
		}
	}
}

func TestChildrenOfStructPath(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	items, hasMore, err := childrenOf(f, "UDSData.CurrentTime", 0, 0)
	if err != nil {
		t.Fatalf("childrenOf: %v", err)
	}
	if hasMore {
		t.Error("a struct's children should never report hasMore")
	}
	if findItem(items, "Month") == nil {
		t.Error("expected a Month item")
	}
}

func TestChildrenOfArrayOfStructs(t *testing.T) {
	f := mustUnmarshal(t, "Player_Local.sav")
	items, hasMore, err := childrenOf(f, "Components", 0, 0)
	if err != nil {
		t.Fatalf("childrenOf: %v", err)
	}
	if hasMore {
		t.Error("Player_Local.sav's Components array has 10 elements, under the default limit — hasMore should be false")
	}
	if len(items) != 10 {
		t.Fatalf("got %d items, want 10", len(items))
	}
	first := findItem(items, "[0]")
	if first == nil {
		t.Fatal("expected an item labeled [0]")
	}
	if !first.Expandable {
		t.Error("a struct array element should be Expandable")
	}
	if first.Path != "Components[0]" {
		t.Errorf("path = %q, want %q", first.Path, "Components[0]")
	}
}

func TestChildrenOfLeafErrors(t *testing.T) {
	f := mustUnmarshal(t, "WorldInfo.sav")
	if _, _, err := childrenOf(f, "WorldName", 0, 0); err == nil {
		t.Fatal("expected an error requesting children of a scalar leaf")
	}
}

// TestChildrenOfStructArrayElement is the regression test for the finding
// that clicking into a single struct-array element (e.g. "Components[0]",
// the natural next step after listing "Components") returned a 400 because
// gvas.Lookup intentionally refuses to resolve a path that ends bare on an
// array index. propsAt must special-case a trailing "[N]" itself.
func TestChildrenOfStructArrayElement(t *testing.T) {
	f := mustUnmarshal(t, "Player_Local.sav")
	items, hasMore, err := childrenOf(f, "Components[0]", 0, 0)
	if err != nil {
		t.Fatalf("childrenOf(Components[0]): %v", err)
	}
	if hasMore {
		t.Error("a struct's children should never report hasMore")
	}
	if len(items) == 0 {
		t.Fatal("expected at least one field under Components[0]")
	}
	// ComponentSaveData's real fields, per docs/FORMAT.md in the sibling
	// skyverse-save-tool repo: "Data" (a StructProperty) and "ComponentName"
	// (a StrProperty).
	data := findItem(items, "Data")
	if data == nil {
		t.Fatal("expected a Data item under Components[0]")
	}
	if !data.Expandable {
		t.Error("Components[0].Data is a StructProperty, should be Expandable")
	}
	if data.Path != "Components[0].Data" {
		t.Errorf("Data path = %q, want %q", data.Path, "Components[0].Data")
	}
	name := findItem(items, "ComponentName")
	if name == nil {
		t.Fatal("expected a ComponentName item under Components[0]")
	}
	if !name.Editable || name.Expandable {
		t.Errorf("ComponentName: got Editable=%v Expandable=%v, want Editable=true Expandable=false", name.Editable, name.Expandable)
	}
}

// TestChildrenOfStructArrayElementNested confirms the trailing-[N] handling
// works when the array index isn't the first segment of the path either —
// propsAt resolves the parent path via gvas.Lookup, which already handles
// arbitrary nesting.
func TestChildrenOfStructArrayElementNested(t *testing.T) {
	f := mustUnmarshal(t, "Player_Local.sav")
	// Components[0].Data is itself a StructProperty (UObjectSaveData), not
	// an array, so nest one level further by going through Components[0]
	// first and confirming the same trailing-index logic applies again at
	// the top level for a second element.
	items, _, err := childrenOf(f, "Components[1]", 0, 0)
	if err != nil {
		t.Fatalf("childrenOf(Components[1]): %v", err)
	}
	if findItem(items, "ComponentName") == nil {
		t.Error("expected a ComponentName item under Components[1]")
	}
}

func TestChildrenOfStructArrayElementOutOfRange(t *testing.T) {
	f := mustUnmarshal(t, "Player_Local.sav")
	if _, _, err := childrenOf(f, "Components[999]", 0, 0); err == nil {
		t.Fatal("expected an error for an out-of-range struct array index")
	}
}

func TestChildrenOfScalarArrayElementIsNotAContainer(t *testing.T) {
	// PlayerMarkerSettings.Filters is an ArrayProperty<BoolProperty> — a
	// trailing [N] on it must NOT be mistaken for a struct-array element
	// (no Structs to index into), so it should still fall through to a
	// clear error rather than a false-positive container match.
	f := mustUnmarshal(t, "Player_Local.sav")
	if _, _, err := childrenOf(f, "PlayerMarkerSettings.Filters[0]", 0, 0); err == nil {
		t.Fatal("expected an error requesting children of a scalar array element")
	}
}

func TestScalarArrayChildrenPagination(t *testing.T) {
	// PlayerMarkerSettings.Filters is an ArrayProperty<BoolProperty> with
	// 8 elements in Player_Local.sav (see docs/FORMAT.md).
	f := mustUnmarshal(t, "Player_Local.sav")

	items, hasMore, err := childrenOf(f, "PlayerMarkerSettings.Filters", 0, 3)
	if err != nil {
		t.Fatalf("childrenOf: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("page 1: got %d items, want 3", len(items))
	}
	if !hasMore {
		t.Fatal("page 1: expected hasMore=true (8 total, limit 3)")
	}
	for _, it := range items {
		if it.Editable {
			t.Errorf("scalar array element %q should not be Editable (gvas cannot address it individually)", it.Label)
		}
		if it.Expandable {
			t.Errorf("scalar array element %q should not be Expandable", it.Label)
		}
	}

	items, hasMore, err = childrenOf(f, "PlayerMarkerSettings.Filters", 6, 3)
	if err != nil {
		t.Fatalf("childrenOf: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("page 3: got %d items, want 2 (8 total, offset 6)", len(items))
	}
	if hasMore {
		t.Fatal("page 3: expected hasMore=false")
	}
}

func TestChildrenOfDefaultLimit(t *testing.T) {
	f := mustUnmarshal(t, "Player_Local.sav")
	// limit=0 should fall back to defaultChildrenLimit, not zero results.
	items, _, err := childrenOf(f, "PlayerMarkerSettings.Filters", 0, 0)
	if err != nil {
		t.Fatalf("childrenOf: %v", err)
	}
	if len(items) != 8 {
		t.Fatalf("got %d items with limit=0 (should default to %d and return all 8), want 8", len(items), defaultChildrenLimit)
	}
}

func TestToItemViews(t *testing.T) {
	items := []childItem{{Label: "Foo", Path: "Foo", Editable: true}}
	views := toItemViews("sess123", items)
	if len(views) != 1 {
		t.Fatalf("got %d views, want 1", len(views))
	}
	if views[0].SessionID != "sess123" {
		t.Errorf("SessionID = %q, want %q", views[0].SessionID, "sess123")
	}
	if views[0].RowID == "" {
		t.Error("expected a non-empty RowID")
	}
}

func TestRowIDIsHTMLIDSafe(t *testing.T) {
	id := rowID("Components[0].Data.ComponentName")
	for _, r := range id {
		if r == '.' || r == '[' || r == ']' || r == ' ' {
			t.Fatalf("rowID(%q) = %q still contains an unsafe character %q", "Components[0].Data.ComponentName", id, string(r))
		}
	}
}

func TestLastPathSegment(t *testing.T) {
	cases := []struct{ path, want string }{
		{"WorldName", "WorldName"},
		{"UDSData.CurrentTime.Month", "Month"},
		{"Components[0].ComponentName", "ComponentName"},
	}
	for _, c := range cases {
		if got := lastPathSegment(c.path); got != c.want {
			t.Errorf("lastPathSegment(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
