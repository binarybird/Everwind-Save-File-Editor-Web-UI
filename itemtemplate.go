package main

import (
	"fmt"

	"github.com/binarybird/Everwind-Save-File-Editor-CLI/gvas"
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
// clearFieldArray empties the named ArrayProperty regardless of its
// InnerType. Only one of ArrayValue's slice fields is ever populated for
// a given array (see gvas/property.go), determined by InnerType -- e.g.
// AdditionalAlchemyEffectsList is an ObjectProperty-inner array (so it
// uses RawCount/RawElements) and AdditionalAlchemyEffectsTiers is a
// ByteProperty-inner array (so it uses Bytes), neither of which is
// Structs. Clearing only Structs left those two as no-ops.
func clearFieldArray(props []*gvas.Property, name string) {
	p := findFieldProp(props, name)
	if p == nil || p.Array == nil {
		return
	}
	p.Array.Structs = nil
	p.Array.Bools = nil
	p.Array.Ints = nil
	p.Array.Int64s = nil
	p.Array.Floats = nil
	p.Array.Doubles = nil
	p.Array.Strings = nil
	p.Array.Bytes = nil
	p.Array.RawCount = 0
	p.Array.RawElements = nil
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
		if p.Native != nil {
			v := *p.Native
			cp.Native = &v
		}
		if p.NestedFile != nil {
			cp.NestedFile = &gvas.File{
				HeaderByte: p.NestedFile.HeaderByte,
				Root:       deepCopyProps(p.NestedFile.Root),
				Footer:     append([]byte(nil), p.NestedFile.Footer...),
			}
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
