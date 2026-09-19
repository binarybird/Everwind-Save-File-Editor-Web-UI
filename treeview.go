package main

import (
	"fmt"
	"strconv"
	"strings"

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
	// gvas.Lookup intentionally refuses to resolve a path that ends bare on
	// an array index (e.g. "Components[0]") — it always resolves to a
	// single *gvas.Property, and there's no single Property for a struct
	// array element itself, only for its fields. Handle that last "[N]"
	// step ourselves: resolve the parent path (which does NOT end in a
	// bare index, so gvas.Lookup handles it fine, at any nesting depth),
	// then index directly into its Structs.
	if parent, index, ok := splitTrailingIndex(path); ok {
		parentProp, err := gvas.Lookup(f, parent)
		if err != nil {
			return nil, false, err
		}
		if parentProp.Array == nil || parentProp.Array.Structs == nil {
			return nil, false, fmt.Errorf("treeview: path %q: [%d] used on a non-struct-array", path, index)
		}
		if index < 0 || index >= len(parentProp.Array.Structs) {
			return nil, false, fmt.Errorf("treeview: path %q: index %d out of range (array has %d elements)", path, index, len(parentProp.Array.Structs))
		}
		return parentProp.Array.Structs[index], true, nil
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

// splitTrailingIndex splits path into everything before a trailing "[N]"
// and the integer N, for the one case gvas.Lookup can't resolve on its own:
// a path that ends bare on a struct-array index. Returns ok=false (and
// leaves parent/index unset) for any path that doesn't end in "[<digits>]" —
// callers fall through to the normal gvas.Lookup-based resolution.
func splitTrailingIndex(path string) (parent string, index int, ok bool) {
	if path == "" || path[len(path)-1] != ']' {
		return "", 0, false
	}
	open := strings.LastIndexByte(path, '[')
	if open < 0 {
		return "", 0, false
	}
	digits := path[open+1 : len(path)-1]
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return "", 0, false
	}
	return path[:open], n, true
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

const defaultChildrenLimit = 100

// childrenOf resolves path against f (empty path = the file's root list)
// and returns a renderable, possibly-paginated list of its children. See
// docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md,
// "Tree-walking helper".
func childrenOf(f *gvas.File, path string, offset, limit int) (items []childItem, hasMore bool, err error) {
	if limit <= 0 {
		limit = defaultChildrenLimit
	}
	props, isContainer, err := propsAt(f, path)
	if err != nil {
		return nil, false, err
	}
	if isContainer {
		return structChildren(props, path), false, nil
	}

	p, err := gvas.Lookup(f, path)
	if err != nil {
		return nil, false, err
	}
	if p.Array == nil {
		return nil, false, fmt.Errorf("treeview: path %q is a leaf, not a container", path)
	}
	if p.Array.Structs != nil {
		return arrayStructChildren(p.Array, path), false, nil
	}
	return scalarArrayChildren(p.Array, path, offset, limit)
}

func arrayStructChildren(a *gvas.ArrayValue, basePath string) []childItem {
	items := make([]childItem, 0, len(a.Structs))
	for i := range a.Structs {
		items = append(items, childItem{
			Label:      fmt.Sprintf("[%d]", i),
			Type:       "StructProperty",
			Path:       fmt.Sprintf("%s[%d]", basePath, i),
			Expandable: true,
		})
	}
	return items
}

// scalarArrayChildren lists a page of a primitive-typed array's elements.
// These are display-only: gvas.Lookup cannot resolve a path into a
// non-struct array element, so scalar array elements are never Editable
// or Expandable.
func scalarArrayChildren(a *gvas.ArrayValue, basePath string, offset, limit int) (items []childItem, hasMore bool, err error) {
	n := arrayLen(a)
	if offset < 0 || offset > n {
		return nil, false, fmt.Errorf("treeview: offset %d out of range (array has %d elements)", offset, n)
	}
	end := offset + limit
	if end > n {
		end = n
	}
	for i := offset; i < end; i++ {
		items = append(items, childItem{
			Label:   fmt.Sprintf("[%d]", i),
			Type:    a.InnerType.Value,
			Path:    fmt.Sprintf("%s[%d]", basePath, i),
			Preview: scalarArrayElementPreview(a, i),
		})
	}
	return items, end < n, nil
}

func scalarArrayElementPreview(a *gvas.ArrayValue, i int) string {
	switch {
	case a.Bools != nil:
		return fmt.Sprintf("%v", a.Bools[i])
	case a.Ints != nil:
		return fmt.Sprintf("%d", a.Ints[i])
	case a.Int64s != nil:
		return fmt.Sprintf("%d", a.Int64s[i])
	case a.Floats != nil:
		return fmt.Sprintf("%g", a.Floats[i])
	case a.Doubles != nil:
		return fmt.Sprintf("%g", a.Doubles[i])
	case a.Strings != nil:
		return a.Strings[i]
	case a.Bytes != nil:
		return fmt.Sprintf("%d", a.Bytes[i])
	default:
		return fmt.Sprintf("<%d raw bytes>", len(a.RawElements))
	}
}

// itemView adds per-request rendering context (which session this belongs
// to, a stable HTML id, an edit-error message) to a childItem, so
// templates never need to reach outside their own data (see
// docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md's
// "Templates" note on why the design avoids a text/template "dict" helper).
type itemView struct {
	childItem
	SessionID string
	RowID     string
	Error     string
}

// ChildrenView is what children.html.tmpl's "children" template renders.
type ChildrenView struct {
	SessionID   string
	Items       []itemView
	HasMore     bool
	LoadMoreURL string
}

func toItemViews(sessionID string, items []childItem) []itemView {
	views := make([]itemView, len(items))
	for i, it := range items {
		views[i] = itemView{childItem: it, SessionID: sessionID, RowID: rowID(it.Path)}
	}
	return views
}

var rowIDReplacer = strings.NewReplacer(".", "-", "[", "-", "]", "", " ", "-")

// rowID derives an HTML-id-safe string from a property path so each leaf
// row can carry a stable `id="leaf-<rowID>"` for htmx to target.
func rowID(path string) string {
	return rowIDReplacer.Replace(path)
}

// lastPathSegment returns the trailing field/index label of a path, for
// display when only the path (not the originating childItem) is on hand
// (e.g. after an edit-handler error where the property couldn't even be
// looked up).
func lastPathSegment(path string) string {
	base := path
	if i := strings.LastIndexByte(base, '.'); i >= 0 {
		base = base[i+1:]
	}
	return base
}
