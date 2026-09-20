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
	if invSlots.Array == nil {
		return nil, fmt.Errorf("inventory Slots: not an array")
	}
	if len(invSlots.Array.Structs) != 63 {
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
	if eqSlots.Array == nil {
		return nil, fmt.Errorf("equipment Slots: not an array")
	}
	if len(eqSlots.Array.Structs) != 8 {
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
