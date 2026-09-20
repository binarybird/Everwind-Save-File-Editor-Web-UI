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

func TestBuildInventoryGridViewNonArraySlotsDoesNotPanic(t *testing.T) {
	// Create a minimal file with a component where Slots is a property with Array == nil.
	// This tests that BuildInventoryGridView returns an error cleanly instead of panicking
	// when encountering a malformed save file (Slots property exists but is not an array).
	slotsProperty := &gvas.Property{
		Name:  "Slots",
		Type:  "ArrayProperty",
		Array: nil, // Malformed: property exists but Array is nil
	}
	dataDataStruct := []*gvas.Property{slotsProperty}
	dataProperty := &gvas.Property{
		Name:   "Data",
		Type:   "StructProperty",
		Struct: dataDataStruct,
	}
	dataPropertyOuter := &gvas.Property{
		Name:   "Data",
		Type:   "StructProperty",
		Struct: []*gvas.Property{dataProperty},
	}
	componentNameProp := &gvas.Property{
		Name: "ComponentName",
		Type: "StrProperty",
		Str:  func() *string { s := "Inventory"; return &s }(),
	}
	componentStruct := []*gvas.Property{componentNameProp, dataPropertyOuter}
	componentsProperty := &gvas.Property{
		Name:  "Components",
		Type:  "ArrayProperty",
		Array: &gvas.ArrayValue{Structs: [][]*gvas.Property{componentStruct}},
	}

	f := &gvas.File{
		Root: []*gvas.Property{componentsProperty},
	}

	view, err := BuildInventoryGridView(f, "sess1", map[string]catalogEntry{})
	if view != nil {
		t.Errorf("expected nil view, got non-nil")
	}
	if err == nil {
		t.Fatal("expected error for nil Array, got nil")
	}
	// If we got here without panicking, the fix is working. If the code had the bug,
	// it would panic when calling arrayLen on a nil Array.
}
