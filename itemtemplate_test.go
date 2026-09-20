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
