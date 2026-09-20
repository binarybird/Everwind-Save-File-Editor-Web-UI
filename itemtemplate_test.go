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

// TestDeepCopyPropsDoesNotAliasNestedStructArrayField exercises the
// Struct/Array recursion in deepCopyProps (not just a top-level scalar):
// it deep-copies the real weapon at Inventory Slots[3], which has a
// 3-element CustomModificators array of structs, mutates the COPY's
// CustomModificators[0].ModifyValue field in place (through its
// pointer, not by replacing the pointer), and asserts the ORIGINAL
// item's corresponding field -- reached independently via a fresh
// gvas.Lookup -- is unaffected. If deepCopyProps ever again left a
// nested pointer aliased (as it did for Property.Native and
// Property.NestedFile before this fix), this test would catch it for
// the Struct/Array path.
func TestDeepCopyPropsDoesNotAliasNestedStructArrayField(t *testing.T) {
	f, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	slotsPath, err := componentSlotsPath(f, "Inventory")
	if err != nil {
		t.Fatalf("componentSlotsPath: %v", err)
	}
	slots, err := gvas.Lookup(f, slotsPath)
	if err != nil || slots.Array == nil {
		t.Fatalf("Lookup(%s): %v", slotsPath, err)
	}
	const weaponSlot = 3 // Inventory Slots[3]: the long sword, CustomModificators has 3 elements.
	if weaponSlot >= len(slots.Array.Structs) {
		t.Fatalf("save has only %d inventory slots, want at least %d", len(slots.Array.Structs), weaponSlot+1)
	}
	itemsProp := findFieldProp(slots.Array.Structs[weaponSlot], "Items")
	if itemsProp == nil || itemsProp.Array == nil || len(itemsProp.Array.Structs) == 0 {
		t.Fatalf("Inventory Slots[%d] has no item", weaponSlot)
	}
	originalItem := itemsProp.Array.Structs[0]

	originalMods := findFieldProp(originalItem, "CustomModificators")
	if originalMods == nil || originalMods.Array == nil || len(originalMods.Array.Structs) == 0 {
		t.Fatalf("Inventory Slots[%d] item has no CustomModificators", weaponSlot)
	}
	originalModVal := findFieldProp(originalMods.Array.Structs[0], "ModifyValue")
	if originalModVal == nil || originalModVal.Float32 == nil {
		t.Fatalf("CustomModificators[0].ModifyValue = %+v, want a Float32", originalModVal)
	}
	before := *originalModVal.Float32

	copiedItem := deepCopyProps(originalItem)
	copiedMods := findFieldProp(copiedItem, "CustomModificators")
	if copiedMods == nil || copiedMods.Array == nil || len(copiedMods.Array.Structs) == 0 {
		t.Fatalf("deep-copied item's CustomModificators = %+v, want 3 elements", copiedMods)
	}
	copiedModVal := findFieldProp(copiedMods.Array.Structs[0], "ModifyValue")
	if copiedModVal == nil || copiedModVal.Float32 == nil {
		t.Fatalf("copied CustomModificators[0].ModifyValue = %+v, want a Float32", copiedModVal)
	}

	// Mutate through the pointer (not by replacing it) so this only
	// passes if deepCopyProps allocated a genuinely distinct float32
	// for the copy's nested struct-array element.
	*copiedModVal.Float32 = before + 1000

	if *originalModVal.Float32 != before {
		t.Fatalf("mutating the copy's nested CustomModificators[0].ModifyValue changed the original: got %v, want unchanged %v -- deepCopyProps aliased a nested struct/array pointer", *originalModVal.Float32, before)
	}
}
