package main

import (
	"fmt"

	"skyversesave/gvas"
)

// childItem is the display/navigation data for one property in the tree,
// independent of which session it came from. See
// docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md,
// "Tree-walking helper".
type childItem struct {
	Label      string // field name, or "[N]" for an array element
	Type       string // Property.Type, or the scalar array's element type
	Preview    string // short value string for a leaf/native/array-summary; empty for plain struct containers
	Path       string // full property path to pass back into propsAt/childrenOf/gvas.Lookup
	Expandable bool
	Editable   bool // true only for the six scalar kinds gvas.Property.Set* supports
}

func joinPath(base, next string) string {
	if base == "" {
		return next
	}
	return base + "." + next
}

// propsAt resolves path (empty means the file's root list) and, if it is
// struct-shaped (the root, a non-native StructProperty, or a NestedFile's
// root), returns its property list with isContainer=true. Anything else
// (an ArrayProperty, a scalar leaf, a native struct) returns
// isContainer=false with a nil slice — the caller falls through to
// array/leaf handling. A StructProperty with zero fields correctly still
// reports isContainer=true with a nil (zero-length) props slice, since
// the discriminator is p.Type, not whether p.Struct happens to be nil.
func propsAt(f *gvas.File, path string) (props []*gvas.Property, isContainer bool, err error) {
	if path == "" {
		return f.Root, true, nil
	}
	p, err := gvas.Lookup(f, path)
	if err != nil {
		return nil, false, err
	}
	if p.Type == "StructProperty" && p.Native == nil {
		return p.Struct, true, nil
	}
	if p.NestedFile != nil {
		return p.NestedFile.Root, true, nil
	}
	return nil, false, nil
}

func structChildren(props []*gvas.Property, basePath string) []childItem {
	items := make([]childItem, 0, len(props))
	for _, p := range props {
		items = append(items, propertyToItem(p, joinPath(basePath, p.Name)))
	}
	return items
}

// propertyToItem describes one property for display, without regard to
// which container it came from.
func propertyToItem(p *gvas.Property, path string) childItem {
	if p.Type == "StructProperty" {
		if p.Native != nil {
			return childItem{Label: p.Name, Type: p.Type, Path: path, Preview: formatNative(p.Native)}
		}
		return childItem{Label: p.Name, Type: p.Type, Path: path, Expandable: true}
	}
	if p.NestedFile != nil {
		return childItem{Label: p.Name, Type: p.Type, Path: path, Expandable: true}
	}
	if p.Array != nil {
		return childItem{Label: p.Name, Type: p.Type, Path: path, Expandable: true, Preview: arraySummary(p.Array)}
	}
	if preview, ok := scalarPreview(p); ok {
		return childItem{Label: p.Name, Type: p.Type, Path: path, Preview: preview, Editable: true}
	}
	if p.Byte != nil {
		return childItem{Label: p.Name, Type: p.Type, Path: path, Preview: fmt.Sprintf("%d", *p.Byte)}
	}
	return childItem{Label: p.Name, Type: p.Type, Path: path, Preview: fmt.Sprintf("<%d raw bytes>", len(p.Raw))}
}

// scalarPreview returns the formatted value and true only for the six
// scalar kinds gvas.Property.Set* supports (see skyverse-save-tool's
// gvas/encode.go) — this is also what determines childItem.Editable.
func scalarPreview(p *gvas.Property) (string, bool) {
	switch {
	case p.Str != nil:
		return *p.Str, true
	case p.Bool != nil:
		return fmt.Sprintf("%v", *p.Bool), true
	case p.Int32 != nil:
		return fmt.Sprintf("%d", *p.Int32), true
	case p.Int64 != nil:
		return fmt.Sprintf("%d", *p.Int64), true
	case p.Float32 != nil:
		return fmt.Sprintf("%g", *p.Float32), true
	case p.Float64 != nil:
		return fmt.Sprintf("%g", *p.Float64), true
	default:
		return "", false
	}
}

func formatNative(n *gvas.NativeValue) string {
	switch {
	case n.Vector != nil:
		return fmt.Sprintf("{%g, %g, %g}", n.Vector.X, n.Vector.Y, n.Vector.Z)
	case n.IntVector != nil:
		return fmt.Sprintf("{%d, %d, %d}", n.IntVector.X, n.IntVector.Y, n.IntVector.Z)
	case n.Rotator != nil:
		return fmt.Sprintf("{pitch:%g yaw:%g roll:%g}", n.Rotator.Pitch, n.Rotator.Yaw, n.Rotator.Roll)
	case n.DateTimeTicks != nil:
		return fmt.Sprintf("%d ticks", *n.DateTimeTicks)
	case n.TimespanTicks != nil:
		return fmt.Sprintf("%d ticks", *n.TimespanTicks)
	case n.Color != nil:
		return fmt.Sprintf("rgba(%d,%d,%d,%d)", n.Color.R, n.Color.G, n.Color.B, n.Color.A)
	case n.LinearColor != nil:
		return fmt.Sprintf("rgba(%g,%g,%g,%g)", n.LinearColor.R, n.LinearColor.G, n.LinearColor.B, n.LinearColor.A)
	default:
		return fmt.Sprintf("<%d raw bytes>", len(n.Raw))
	}
}

func arraySummary(a *gvas.ArrayValue) string {
	return fmt.Sprintf("%d elements", arrayLen(a))
}

func arrayLen(a *gvas.ArrayValue) int {
	switch {
	case a.Structs != nil:
		return len(a.Structs)
	case a.Bools != nil:
		return len(a.Bools)
	case a.Ints != nil:
		return len(a.Ints)
	case a.Int64s != nil:
		return len(a.Int64s)
	case a.Floats != nil:
		return len(a.Floats)
	case a.Doubles != nil:
		return len(a.Doubles)
	case a.Strings != nil:
		return len(a.Strings)
	case a.Bytes != nil:
		return len(a.Bytes)
	default:
		return int(a.RawCount)
	}
}
