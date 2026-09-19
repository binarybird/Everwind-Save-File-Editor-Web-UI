# Skyverse Save Web Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A standalone Go web server, importing `skyverse-save-tool`'s `gvas` library, that lets you upload a Skyverse `.sav` file, browse its property tree in a lazy-expanding htmx UI, edit scalar values, and download the edited file.

**Architecture:** A single Go binary (`net/http` + Go's 1.22+ method/wildcard `ServeMux`, no third-party router) serves one embedded HTML page and a handful of endpoints that render server-side HTML fragments (via `html/template`) for htmx to swap into the page — no client-side JS beyond the vendored htmx library itself. An in-memory session store holds each uploaded file's decoded `*gvas.File` for the life of the process; edits mutate that in-memory tree directly via `gvas`'s existing `Lookup`/`Set*` API, and downloading re-`Marshal`s it.

**Tech Stack:** Go stdlib only on the server side (`net/http`, `html/template`, `embed`), htmx (vendored client-side script, not a Go dependency) on the client side, `skyversesave/gvas` (the sibling `skyverse-save-tool` project) as the only Go module dependency.

**Spec:** `docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md`

## Global Constraints

- No third-party Go dependencies — only `skyversesave/gvas` (a local sibling module) and the Go standard library.
- This project only *consumes* `gvas`'s exported API (`Unmarshal`, `Marshal`, `Lookup`, `Property`/`File`/`ArrayValue`/`NativeValue` fields, `SetString`/`SetBool`/`SetInt32`/`SetInt64`/`SetFloat32`/`SetFloat64`) — it never modifies `skyverse-save-tool`.
- htmx is vendored locally (embedded into the binary via `go:embed`) — the server must work fully offline, no CDN dependency at runtime.
- Editing is scoped to the same six scalar kinds `gvas.Property.Set*` supports — no adding/removing properties or array elements, and no per-element editing of scalar arrays (`gvas.Lookup` cannot resolve a path into a non-struct array element at all — only `ArrayProperty<StructProperty>` elements are addressable).
- Sessions are in-memory only, keyed by a random token embedded in every session-scoped URL — no cookies, no auth, no persistence beyond process lifetime (explicitly out of scope per the spec).
- Uploads capped at 50MB via `http.MaxBytesReader`.
- The three real save files from `skyverse-save-tool/testdata/` are copied into this project's own `testdata/` for handler tests.

---

## Task 1: Project scaffolding

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `testdata/Player_Local.sav`, `testdata/Player_Remote_021fa7902e50eeeb8c6ebdbe4fc922fe.sav`, `testdata/WorldInfo.sav` (copy from `../skyverse-save-tool/testdata/`)

**Interfaces:**
- Produces: module `skyverseweb`, requiring `skyversesave` via a local `replace` directive pointing at `../skyverse-save-tool`; a `main()` that starts an HTTP server on a configurable address and serves a placeholder response at `/`.

- [ ] **Step 1: Copy the test fixtures**

Run:
```bash
cd /home/binarybird/Desktop/analysis/skyverse-save-web
mkdir -p testdata
cp ../skyverse-save-tool/testdata/Player_Local.sav testdata/
cp ../skyverse-save-tool/testdata/Player_Remote_021fa7902e50eeeb8c6ebdbe4fc922fe.sav testdata/
cp ../skyverse-save-tool/testdata/WorldInfo.sav testdata/
```

Expected: three `.sav` files now present under `testdata/`.

- [ ] **Step 2: Initialize the module and add the local dependency**

Run:
```bash
go mod init skyverseweb
go mod edit -replace skyversesave=../skyverse-save-tool
go mod edit -require skyversesave@v0.0.0
go mod tidy
```

Expected: `go.mod` now contains a `require skyversesave v0.0.0-...` line (tidy will fill in a pseudo-version) and a `replace skyversesave => ../skyverse-save-tool` line. `go.sum` may or may not be created (local replaces to a module with no external deps of its own often don't need one — that's fine either way).

- [ ] **Step 3: Write a minimal main.go**

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "skyverse-save-web placeholder")
	})

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
```

- [ ] **Step 4: Verify it builds and runs**

Run:
```bash
go build ./...
./skyverseweb -addr :18080 &
sleep 0.3
curl -s http://localhost:18080/
kill %1
```

Expected: `curl` prints `skyverse-save-web placeholder`.

- [ ] **Step 5: Add a .gitignore and commit**

```bash
cat > .gitignore <<'EOF'
/skyverseweb
EOF
git add go.mod go.sum main.go testdata .gitignore
git commit -m "chore: scaffold Go module, depend on local gvas via replace directive"
```

(If `go.sum` wasn't created in Step 2, omit it from `git add` — `git add` on a nonexistent path is harmless to skip.)

---

## Task 2: Session store

**Files:**
- Create: `session.go`
- Test: `session_test.go`

**Interfaces:**
- Consumes: `gvas.File` (from the `skyversesave/gvas` package).
- Produces: `type Session struct { File *gvas.File }`, `type SessionStore struct { ... }`, `func NewSessionStore() *SessionStore`, `func (s *SessionStore) Create(f *gvas.File) (string, error)`, `func (s *SessionStore) Get(id string) (*Session, bool)`.

- [ ] **Step 1: Write the failing test**

Create `session_test.go`:

```go
package main

import (
	"testing"

	"skyversesave/gvas"
)

func TestSessionStoreCreateAndGet(t *testing.T) {
	store := NewSessionStore()
	f := &gvas.File{}

	id, err := store.Create(f)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned an empty id")
	}

	got, ok := store.Get(id)
	if !ok {
		t.Fatal("Get: session not found")
	}
	if got.File != f {
		t.Error("Get returned a different *gvas.File than was stored")
	}
}

func TestSessionStoreGetUnknownID(t *testing.T) {
	store := NewSessionStore()
	if _, ok := store.Get("does-not-exist"); ok {
		t.Fatal("expected ok=false for an unknown session id")
	}
}

func TestSessionStoreCreateReturnsUniqueIDs(t *testing.T) {
	store := NewSessionStore()
	f := &gvas.File{}

	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := store.Create(f)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate session id generated: %s", id)
		}
		seen[id] = true
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestSessionStore -v`
Expected: FAIL — `NewSessionStore` undefined.

- [ ] **Step 3: Implement session.go**

```go
package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"skyversesave/gvas"
)

// Session holds one uploaded save's decoded tree for the life of the
// server process. See docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md,
// "Session model" — no persistence, no TTL, in-memory only.
type Session struct {
	File *gvas.File
}

// SessionStore is a concurrency-safe in-memory map of session id -> Session.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

// Create stores f under a freshly minted random id and returns that id.
func (s *SessionStore) Create(f *gvas.File) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = &Session{File: f}
	return id, nil
}

func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestSessionStore -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add session.go session_test.go
git commit -m "feat: in-memory session store"
```

---

## Task 3: Tree-walking helper — struct/native/array item rendering

**Files:**
- Create: `treeview.go`
- Test: `treeview_test.go`

**Interfaces:**
- Consumes: `skyversesave/gvas`'s `File`, `Property`, `ArrayValue`, `NativeValue`, `Lookup`.
- Produces:
  - `type childItem struct { Label, Type, Preview, Path string; Expandable, Editable bool }`
  - `func propsAt(f *gvas.File, path string) (props []*gvas.Property, isContainer bool, err error)`
  - `func structChildren(props []*gvas.Property, basePath string) []childItem`
  - `func propertyToItem(p *gvas.Property, path string) childItem`
  - `func scalarPreview(p *gvas.Property) (string, bool)`
  - `func formatNative(n *gvas.NativeValue) string`
  - `func arraySummary(a *gvas.ArrayValue) string`
  - `func arrayLen(a *gvas.ArrayValue) int`
  - `func joinPath(base, next string) string`

This task covers everything needed to list the children of a *struct-shaped* node (the file root, a non-native `StructProperty`, or a `NestedFile`'s root) and to describe one property as a `childItem`. Array-shaped nodes (indexing into an `ArrayProperty`) are Task 4.

- [ ] **Step 1: Write the failing tests**

Create `treeview_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail to compile**

Run: `go test ./... -run 'PropsAt|StructChildren|JoinPath' -v`
Expected: FAIL — the functions/types don't exist yet.

- [ ] **Step 3: Implement treeview.go**

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'PropsAt|StructChildren|JoinPath' -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add treeview.go treeview_test.go
git commit -m "feat: tree-walking helper for struct/native/array child rendering"
```

---

## Task 4: Tree-walking helper — array children, pagination, and view models

**Files:**
- Modify: `treeview.go`
- Modify: `treeview_test.go`

**Interfaces:**
- Consumes: `childItem`, `propsAt`, `arrayLen`, `joinPath` (Task 3).
- Produces:
  - `const defaultChildrenLimit = 100`
  - `func childrenOf(f *gvas.File, path string, offset, limit int) (items []childItem, hasMore bool, err error)`
  - `func arrayStructChildren(a *gvas.ArrayValue, basePath string) []childItem`
  - `func scalarArrayChildren(a *gvas.ArrayValue, basePath string, offset, limit int) (items []childItem, hasMore bool, err error)`
  - `func scalarArrayElementPreview(a *gvas.ArrayValue, i int) string`
  - `type itemView struct { childItem; SessionID, RowID, Error string }`
  - `type ChildrenView struct { SessionID string; Items []itemView; HasMore bool; LoadMoreURL string }`
  - `func toItemViews(sessionID string, items []childItem) []itemView`
  - `func rowID(path string) string`
  - `func lastPathSegment(path string) string`

- [ ] **Step 1: Write the failing tests**

Append to `treeview_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'ChildrenOf|ScalarArray|ToItemViews|RowID|LastPathSegment' -v`
Expected: FAIL — `childrenOf` and friends don't exist yet.

- [ ] **Step 3: Append the implementation to treeview.go**

```go
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
```

Add `"strings"` to the `import` block at the top of `treeview.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: all PASS (Tasks 2-4 together).

- [ ] **Step 5: Commit**

```bash
git add treeview.go treeview_test.go
git commit -m "feat: array children, pagination, and per-request view models"
```

---

## Task 5: Templates and static assets

**Files:**
- Create: `templates/index.html.tmpl`
- Create: `templates/children.html.tmpl`
- Create: `static/style.css`
- Create: `static/htmx.min.js` (vendored)

**Interfaces:**
- Produces: three named templates (`index.html.tmpl`, `children`, `leafRow`, `uploadResponse`) that Task 6's server wiring parses via `template.ParseFS`.

- [ ] **Step 1: Vendor htmx**

Run:
```bash
mkdir -p static
curl -fsSL -o static/htmx.min.js https://unpkg.com/htmx.org@1.9.12/dist/htmx.min.js
```

Expected: `static/htmx.min.js` exists and is non-trivially sized (htmx 1.9.12's minified build is roughly 45KB). Verify with:
```bash
wc -c static/htmx.min.js
head -c 200 static/htmx.min.js
```
If this environment has no outbound network access, the `curl` will fail — if so, report that as a concern (DONE_WITH_CONCERNS) rather than fabricating file content, and leave `static/htmx.min.js` absent for the controller to fetch/provide out of band. Do not hand-write a fake or placeholder htmx implementation.

- [ ] **Step 2: Write static/style.css**

```css
body {
  font-family: system-ui, sans-serif;
  max-width: 900px;
  margin: 2rem auto;
  padding: 0 1rem;
}

ul {
  list-style: none;
  padding-left: 1.25rem;
}

li {
  margin: 0.15rem 0;
}

.label {
  font-weight: 600;
  margin-right: 0.4rem;
}

.type {
  color: #888;
  font-size: 0.85em;
  margin-right: 0.4rem;
}

.value, .preview {
  color: #333;
}

.toggle {
  cursor: pointer;
  font-weight: 600;
}
.toggle:hover {
  text-decoration: underline;
}

.error {
  color: #b00020;
  margin-left: 0.5rem;
}

.leaf form {
  display: inline;
}
.leaf input[type="text"] {
  width: 16rem;
}

#download-area a {
  display: inline-block;
  margin: 1rem 0;
}
```

- [ ] **Step 3: Write templates/index.html.tmpl**

```html
{{define "index.html.tmpl"}}
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Skyverse Save Viewer</title>
  <script src="/static/htmx.min.js"></script>
  <link rel="stylesheet" href="/static/style.css">
</head>
<body>
  <h1>Skyverse Save Viewer</h1>
  <form hx-post="/upload" hx-target="#tree" hx-swap="innerHTML" hx-encoding="multipart/form-data">
    <input type="file" name="savefile" accept=".sav" required>
    <button type="submit">Upload</button>
  </form>
  <div id="download-area"></div>
  <div id="tree"></div>
</body>
</html>
{{end}}
```

- [ ] **Step 4: Write templates/children.html.tmpl**

```html
{{define "children"}}
<ul>
  {{range .Items}}
    {{if .Editable}}
      {{template "leafRow" .}}
    {{else if .Expandable}}
      <li class="node">
        <span class="toggle"
              hx-get="/session/{{.SessionID}}/children?path={{.Path}}"
              hx-target="next .children"
              hx-swap="innerHTML"
              hx-trigger="click once">{{.Label}}</span>
        <span class="type">{{.Type}}</span>
        {{if .Preview}}<span class="preview">{{.Preview}}</span>{{end}}
        <div class="children"></div>
      </li>
    {{else}}
      <li class="leaf-readonly">
        <span class="label">{{.Label}}</span>
        <span class="type">{{.Type}}</span>
        <span class="value">{{.Preview}}</span>
      </li>
    {{end}}
  {{end}}
  {{if .HasMore}}
    <li class="load-more">
      <button hx-get="{{.LoadMoreURL}}" hx-target="closest ul" hx-swap="outerHTML">Load more…</button>
    </li>
  {{end}}
</ul>
{{end}}

{{define "leafRow"}}
<li class="leaf" id="leaf-{{.RowID}}">
  <span class="label">{{.Label}}</span>
  <span class="type">{{.Type}}</span>
  <form hx-post="/session/{{.SessionID}}/edit" hx-target="#leaf-{{.RowID}}" hx-swap="outerHTML">
    <input type="hidden" name="path" value="{{.Path}}">
    <input type="text" name="value" value="{{.Preview}}">
    <button type="submit">Save</button>
  </form>
  {{if .Error}}<span class="error">{{.Error}}</span>{{end}}
</li>
{{end}}

{{define "uploadResponse"}}
<div id="download-area" hx-swap-oob="true"><a href="/session/{{.SessionID}}/download">Download edited save</a></div>
{{template "children" .}}
{{end}}
```

- [ ] **Step 5: Commit**

```bash
git add templates static
git commit -m "feat: htmx page shell, tree/leaf/upload templates, vendored htmx and stylesheet"
```

---

## Task 6: Server wiring and index handler

**Files:**
- Modify: `main.go`
- Create: `handlers.go`
- Test: `handlers_test.go`

**Interfaces:**
- Consumes: `SessionStore` (Task 2).
- Produces:
  - `type Server struct { store *SessionStore; templates *template.Template }`
  - `func NewServer(store *SessionStore, templates *template.Template) *Server`
  - `func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request)`
  - `func (s *Server) routes() http.Handler`
  - `//go:embed` directives for `templates/` and `static/` in `main.go`, wired into `Server`/`http.ServeMux`.

- [ ] **Step 1: Write the failing test**

Create `handlers_test.go`. This references `templatesFS` and `embeddedStaticFS`, which Step 4 defines in `main.go` — until then, the whole package fails to compile, which is the expected RED state for this task (nothing here compiles until Steps 3 and 4 are both done):

```go
package main

import (
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	return NewServer(NewSessionStore(), tmpl)
}

func TestHandleIndex(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Skyverse Save Viewer") {
		t.Error("index page missing expected title text")
	}
	if !strings.Contains(body, `hx-post="/upload"`) {
		t.Error("index page missing the upload form")
	}
}

func TestStaticAssetsServed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty static/style.css response")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run 'HandleIndex|StaticAssets' -v`
Expected: FAIL to compile — `templatesFS`, `embeddedStaticFS`, `Server`, `NewServer` undefined.

- [ ] **Step 3: Implement handlers.go (index + routing only for now)**

```go
package main

import (
	"html/template"
	"net/http"
)

// Server holds the shared dependencies every handler needs.
type Server struct {
	store     *SessionStore
	templates *template.Template
}

func NewServer(store *SessionStore, templates *template.Template) *Server {
	return &Server{store: store, templates: templates}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if err := s.templates.ExecuteTemplate(w, "index.html.tmpl", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 4: Wire embeds and startup into main.go**

Replace `main.go`'s contents:

```go
package main

import (
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

//go:embed static
var embeddedStaticFS embed.FS

// staticFS strips the "static/" prefix so http.FileServer serves
// static/style.css as /static/style.css rather than /static/static/style.css.
var staticFS fs.FS

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	flag.Parse()

	var err error
	staticFS, err = fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		log.Fatalf("static assets: %v", err)
	}

	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		log.Fatalf("parsing templates: %v", err)
	}

	srv := NewServer(NewSessionStore(), tmpl)

	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.routes()))
}
```

`handlers_test.go`'s `newTestServer` (written in Step 1) already sets `staticFS` and references `templatesFS`/`embeddedStaticFS` — no further test changes needed here now that `main.go` defines them.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: all PASS.

- [ ] **Step 6: Manual smoke test**

Run:
```bash
go build ./...
./skyverseweb -addr :18080 &
sleep 0.3
curl -s http://localhost:18080/ | grep -o "Skyverse Save Viewer"
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:18080/static/htmx.min.js
kill %1
```
Expected: prints `Skyverse Save Viewer`, then `200`.

- [ ] **Step 7: Commit**

```bash
git add main.go handlers.go handlers_test.go
git commit -m "feat: embed templates/static assets, wire index handler and routing"
```

---

## Task 7: Upload handler

**Files:**
- Modify: `handlers.go`
- Modify: `handlers_test.go`

**Interfaces:**
- Consumes: `SessionStore.Create` (Task 2), `ChildrenView`/`toItemViews`/`structChildren` (Tasks 3-4), `gvas.Unmarshal`.
- Produces: `func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request)`, wired as `POST /upload`.

- [ ] **Step 1: Write the failing tests**

Append to `handlers_test.go`:

```go
func TestHandleUploadValidFile(t *testing.T) {
	s := newTestServer(t)
	req := multipartUploadRequest(t, "testdata/WorldInfo.sav")
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "WorldName") {
		t.Error("upload response missing expected top-level property WorldName")
	}
	if !strings.Contains(body, "hx-swap-oob") {
		t.Error("upload response missing the out-of-band download link")
	}
	if !strings.Contains(body, "/download") {
		t.Error("upload response missing a download link")
	}
}

func TestHandleUploadCorruptFile(t *testing.T) {
	s := newTestServer(t)
	req := multipartUploadRequestBytes(t, []byte("not a save file"))
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Error("expected an error message in the response body")
	}
}

func TestHandleUploadNoFile(t *testing.T) {
	s := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestHandleUploadTooLarge(t *testing.T) {
	s := newTestServer(t)
	oversized := make([]byte, maxUploadSize+1)
	req := multipartUploadRequestBytes(t, oversized)
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func multipartUploadRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return multipartUploadRequestBytes(t, data)
}

func multipartUploadRequestBytes(t *testing.T, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("savefile", "upload.sav")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}
```

Add `"bytes"`, `"mime/multipart"`, and `"os"` to `handlers_test.go`'s imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'HandleUpload' -v`
Expected: FAIL — `404` (no `/upload` route yet).

- [ ] **Step 3: Add the upload handler to handlers.go**

```go
const maxUploadSize = 50 << 20 // 50MB, generously above the ~1MB real samples

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("file too large or malformed upload: %w", err))
		return
	}
	file, _, err := r.FormFile("savefile")
	if err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("no file provided: %w", err))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("reading upload: %w", err))
		return
	}

	parsed, err := gvas.Unmarshal(data)
	if err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("this doesn't look like a valid save file: %w", err))
		return
	}

	id, err := s.store.Create(parsed)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("creating session: %w", err))
		return
	}

	items, _, err := childrenOf(parsed, "", 0, 0)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("rendering root: %w", err))
		return
	}
	view := ChildrenView{SessionID: id, Items: toItemViews(id, items)}
	if err := s.templates.ExecuteTemplate(w, "uploadResponse", view); err != nil {
		log.Printf("rendering uploadResponse: %v", err)
	}
}

// renderError writes a small inline error fragment. Used by every handler
// for its failure paths — see docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md,
// "Error handling".
func renderError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	fmt.Fprintf(w, `<div class="error">%s</div>`, template.HTMLEscapeString(err.Error()))
}
```

Add `"fmt"`, `"io"`, `"log"`, and `"skyversesave/gvas"` to `handlers.go`'s imports.

- [ ] **Step 4: Wire the route in routes()**

In `handlers.go`, add to `routes()`:

```go
	mux.HandleFunc("POST /upload", s.handleUpload)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add handlers.go handlers_test.go
git commit -m "feat: upload handler with session creation and root fragment response"
```

---

## Task 8: Children handler

**Files:**
- Modify: `handlers.go`
- Modify: `handlers_test.go`

**Interfaces:**
- Consumes: `childrenOf`, `toItemViews`, `ChildrenView` (Tasks 3-4), `SessionStore.Get` (Task 2).
- Produces: `func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request)`, wired as `GET /session/{id}/children`.

- [ ] **Step 1: Write the failing tests**

Append to `handlers_test.go`:

```go
func uploadAndGetSessionID(t *testing.T, s *Server, testdataFile string) string {
	t.Helper()
	req := multipartUploadRequest(t, testdataFile)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed: status %d, body %s", rec.Code, rec.Body.String())
	}
	// Extract the session id from the download link href="/session/<id>/download".
	body := rec.Body.String()
	const marker = `/session/`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("could not find session id in upload response: %s", body)
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "/")
	if j < 0 {
		t.Fatalf("could not parse session id from: %s", rest)
	}
	return rest[:j]
}

func TestHandleChildrenNestedPath(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=UDSData.CurrentTime", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Month", "Day", "TimeOfDay"} {
		if !strings.Contains(body, want) {
			t.Errorf("children fragment missing expected field %q", want)
		}
	}
}

func TestHandleChildrenPagination(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=PlayerMarkerSettings.Filters&limit=3", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Load more") {
		t.Error("expected a 'Load more' control when more items remain")
	}
}

func TestHandleChildrenUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/children?path=", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "re-upload") {
		t.Error("expected a 'please re-upload' message for an unknown session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'HandleChildren' -v`
Expected: FAIL — no `/session/{id}/children` route yet.

- [ ] **Step 3: Add the children handler to handlers.go**

```go
func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	path := r.URL.Query().Get("path")
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 0)

	items, hasMore, err := childrenOf(sess.File, path, offset, limit)
	if err != nil {
		renderError(w, http.StatusBadRequest, err)
		return
	}

	view := ChildrenView{SessionID: id, Items: toItemViews(id, items)}
	if hasMore {
		nextOffset := offset + limit
		if limit <= 0 {
			nextOffset = offset + defaultChildrenLimit
		}
		view.HasMore = true
		view.LoadMoreURL = fmt.Sprintf("/session/%s/children?path=%s&offset=%d&limit=%d", id, path, nextOffset, limitOrDefault(limit))
	}

	if err := s.templates.ExecuteTemplate(w, "children", view); err != nil {
		log.Printf("rendering children: %v", err)
	}
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func limitOrDefault(limit int) int {
	if limit <= 0 {
		return defaultChildrenLimit
	}
	return limit
}

func renderSessionNotFound(w http.ResponseWriter) {
	renderError(w, http.StatusNotFound, fmt.Errorf("session not found or expired — please re-upload your save file"))
}
```

Add `"strconv"` to `handlers.go`'s imports.

- [ ] **Step 4: Wire the route in routes()**

```go
	mux.HandleFunc("GET /session/{id}/children", s.handleChildren)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add handlers.go handlers_test.go
git commit -m "feat: children handler with pagination and unknown-session handling"
```

---

## Task 9: Edit handler

**Files:**
- Modify: `handlers.go`
- Modify: `handlers_test.go`

**Interfaces:**
- Consumes: `gvas.Lookup`, `Property.SetString/SetBool/SetInt32/SetInt64/SetFloat32/SetFloat64`, `scalarPreview`, `lastPathSegment`, `rowID` (earlier tasks).
- Produces: `func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request)`, `func applyEdit(p *gvas.Property, value string) (newPreview string, err error)`, `func previewOf(p *gvas.Property) string`, wired as `POST /session/{id}/edit`.

- [ ] **Step 1: Write the failing tests**

Append to `handlers_test.go`:

```go
func TestHandleEditString(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := editRequest(t, id, "WorldName", "NewName")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NewName") {
		t.Error("edit response should show the new value")
	}

	sess, _ := s.store.Get(id)
	p, err := gvas.Lookup(sess.File, "WorldName")
	if err != nil || p.Str == nil || *p.Str != "NewName" {
		t.Fatalf("session tree not updated: %+v, err=%v", p, err)
	}
}

func TestHandleEditBadValue(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := editRequest(t, id, "bNewGame", "not-a-bool")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Error("expected an inline error message")
	}

	sess, _ := s.store.Get(id)
	p, err := gvas.Lookup(sess.File, "bNewGame")
	if err != nil || p.Bool == nil || *p.Bool != false {
		t.Fatalf("session tree should be unchanged after a failed edit: %+v, err=%v", p, err)
	}
}

func TestHandleEditNonScalarRejected(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := editRequest(t, id, "UDSData", "whatever")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleEditUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := editRequest(t, "does-not-exist", "WorldName", "X")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func editRequest(t *testing.T, sessionID, path, value string) *http.Request {
	t.Helper()
	form := url.Values{"path": {path}, "value": {value}}
	req := httptest.NewRequest(http.MethodPost, "/session/"+sessionID+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}
```

Add `"net/url"` and `"skyversesave/gvas"` to `handlers_test.go`'s imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'HandleEdit' -v`
Expected: FAIL — no `/session/{id}/edit` route yet.

- [ ] **Step 3: Add the edit handler to handlers.go**

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
		preview = previewOf(p) // unchanged — applyEdit never partially mutates on a parse error
	}

	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "leafRow", itemView{
		childItem: childItem{Label: lastPathSegment(path), Type: p.Type, Path: path, Editable: true, Preview: preview},
		SessionID: id,
		RowID:     rowID(path),
		Error:     errMsg,
	})
}

// applyEdit parses value against p's existing scalar kind and applies it,
// mirroring skyverse-save-tool's cmd/saveview/set.go applyScalarEdit. It
// returns the freshly formatted preview string on success.
func applyEdit(p *gvas.Property, value string) (string, error) {
	switch {
	case p.Str != nil:
		if err := p.SetString(value); err != nil {
			return "", err
		}
		return value, nil
	case p.Bool != nil:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("parsing %q as bool: %w", value, err)
		}
		if err := p.SetBool(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%v", v), nil
	case p.Int32 != nil:
		v, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return "", fmt.Errorf("parsing %q as int32: %w", value, err)
		}
		if err := p.SetInt32(int32(v)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case p.Int64 != nil:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return "", fmt.Errorf("parsing %q as int64: %w", value, err)
		}
		if err := p.SetInt64(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case p.Float32 != nil:
		v, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return "", fmt.Errorf("parsing %q as float32: %w", value, err)
		}
		if err := p.SetFloat32(float32(v)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%g", v), nil
	case p.Float64 != nil:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", fmt.Errorf("parsing %q as float64: %w", value, err)
		}
		if err := p.SetFloat64(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%g", v), nil
	default:
		return "", fmt.Errorf("%q (type %s) is not an editable scalar", p.Name, p.Type)
	}
}

// previewOf formats p's current scalar value, for redisplay after a
// failed edit. Returns "" for a non-scalar property (shouldn't happen —
// callers only use this after applyEdit already confirmed p is scalar-shaped).
func previewOf(p *gvas.Property) string {
	s, _ := scalarPreview(p)
	return s
}
```

- [ ] **Step 4: Wire the route in routes()**

```go
	mux.HandleFunc("POST /session/{id}/edit", s.handleEdit)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add handlers.go handlers_test.go
git commit -m "feat: edit handler with per-type validation and unchanged-on-failure guarantee"
```

---

## Task 10: Download handler

**Files:**
- Modify: `handlers.go`
- Modify: `handlers_test.go`

**Interfaces:**
- Consumes: `gvas.Marshal`, `SessionStore.Get` (Task 2).
- Produces: `func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request)`, wired as `GET /session/{id}/download`.

- [ ] **Step 1: Write the failing tests**

Append to `handlers_test.go`:

```go
func TestHandleDownload(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Error("expected Content-Disposition: attachment")
	}

	f, err := gvas.Unmarshal(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("downloaded bytes don't parse as a save file: %v", err)
	}
	if len(f.Root) == 0 {
		t.Error("downloaded file has no top-level properties")
	}
}

func TestHandleDownloadReflectsEdit(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	editReq := editRequest(t, id, "WorldName", "EditedViaWeb")
	editRec := httptest.NewRecorder()
	s.routes().ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit failed: status %d, body %s", editRec.Code, editRec.Body.String())
	}

	original, err := os.ReadFile("testdata/WorldInfo.sav")
	if err != nil {
		t.Fatal(err)
	}
	originalFile, err := gvas.Unmarshal(original)
	if err != nil {
		t.Fatal(err)
	}

	dlReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	dlRec := httptest.NewRecorder()
	s.routes().ServeHTTP(dlRec, dlReq)

	edited, err := gvas.Unmarshal(dlRec.Body.Bytes())
	if err != nil {
		t.Fatalf("downloaded bytes don't parse: %v", err)
	}
	name := propByName(edited.Root, "WorldName")
	if name == nil || *name.Str != "EditedViaWeb" {
		t.Fatalf("WorldName after edit+download: %+v", name)
	}
	version := propByName(edited.Root, "GameVersion")
	wantVersion := propByName(originalFile.Root, "GameVersion")
	if version == nil || wantVersion == nil || *version.Str != *wantVersion.Str {
		t.Errorf("GameVersion changed unexpectedly: got %+v, want %+v", version, wantVersion)
	}
}

func propByName(props []*gvas.Property, name string) *gvas.Property {
	for _, p := range props {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func TestHandleDownloadUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/download", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'HandleDownload' -v`
Expected: FAIL — no `/session/{id}/download` route yet.

- [ ] **Step 3: Add the download handler to handlers.go**

```go
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	data, err := gvas.Marshal(sess.File)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("encoding save: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="edited.sav"`)
	w.Write(data)
}
```

- [ ] **Step 4: Wire the route in routes()**

```go
	mux.HandleFunc("GET /session/{id}/download", s.handleDownload)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add handlers.go handlers_test.go
git commit -m "feat: download handler streaming the session's current Marshal output"
```

---

## Task 11: Full-flow integration test

**Files:**
- Create: `integration_test.go`

**Interfaces:**
- Consumes: all handlers and `routes()` (Tasks 6-10).

This task doesn't add new production code — it assembles the pieces into
the end-to-end scenarios the spec's Testing section calls for explicitly,
exercised against all three real save files rather than just one.

- [ ] **Step 1: Write the integration tests**

Create `integration_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"skyversesave/gvas"
)

func TestFullFlowUploadBrowseEditDownload(t *testing.T) {
	for _, tf := range []string{
		"testdata/Player_Local.sav",
		"testdata/Player_Remote_021fa7902e50eeeb8c6ebdbe4fc922fe.sav",
		"testdata/WorldInfo.sav",
	} {
		t.Run(tf, func(t *testing.T) {
			s := newTestServer(t)
			id := uploadAndGetSessionID(t, s, tf)

			// Browse: expand the root, then a nested path if this file has one.
			rootReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=", nil)
			rootRec := httptest.NewRecorder()
			s.routes().ServeHTTP(rootRec, rootReq)
			if rootRec.Code != http.StatusOK {
				t.Fatalf("root children: status %d", rootRec.Code)
			}

			// Edit a known top-level bool field present in all three files.
			editReq := editRequest(t, id, "bIsOnIsland", "false")
			editRec := httptest.NewRecorder()
			s.routes().ServeHTTP(editRec, editReq)
			if tf == "testdata/WorldInfo.sav" {
				// WorldInfo.sav has no bIsOnIsland field — expect a clean 400, not a panic.
				if editRec.Code != http.StatusBadRequest {
					t.Fatalf("edit on missing field: status %d, want 400", editRec.Code)
				}
				return
			}
			if editRec.Code != http.StatusOK {
				t.Fatalf("edit: status %d, body %s", editRec.Code, editRec.Body.String())
			}

			// Download and verify the edit landed, and nothing else moved.
			dlReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
			dlRec := httptest.NewRecorder()
			s.routes().ServeHTTP(dlRec, dlReq)
			if dlRec.Code != http.StatusOK {
				t.Fatalf("download: status %d", dlRec.Code)
			}
			edited, err := gvas.Unmarshal(dlRec.Body.Bytes())
			if err != nil {
				t.Fatalf("downloaded file doesn't parse: %v", err)
			}
			p := propByName(edited.Root, "bIsOnIsland")
			if p == nil || p.Bool == nil || *p.Bool != false {
				t.Fatalf("bIsOnIsland after edit: %+v", p)
			}
			island := propByName(edited.Root, "IslandID")
			if island == nil || island.Str == nil || *island.Str == "" {
				t.Error("IslandID should be unchanged and non-empty")
			}
		})
	}
}

func TestFullFlowSessionScopedEndpointsRejectUnknownSession(t *testing.T) {
	s := newTestServer(t)
	unknown := "0000000000000000000000000000000000"

	endpoints := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/session/"+unknown+"/children?path=", nil),
		editRequest(t, unknown, "WorldName", "X"),
		httptest.NewRequest(http.MethodGet, "/session/"+unknown+"/download", nil),
	}
	for _, req := range endpoints {
		rec := httptest.NewRecorder()
		s.routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", req.Method, req.URL.Path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "re-upload") {
			t.Errorf("%s %s: expected a re-upload message", req.Method, req.URL.Path)
		}
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./... -run 'FullFlow' -v`
Expected: all PASS. (These exercise only already-implemented handlers, so no RED phase is expected here — if something fails, it indicates a real integration bug in a prior task, not a missing feature; investigate rather than adding new production code speculatively.)

- [ ] **Step 3: Run the full suite**

Run: `go test ./... -v`
Expected: all PASS (Tasks 2-11 together).

- [ ] **Step 4: Commit**

```bash
git add integration_test.go
git commit -m "test: end-to-end upload/browse/edit/download flow across all three sample files"
```

---

## Task 12: README and final verification pass

**Files:**
- Create: `README.md`

**Interfaces:** none (documentation + verification only).

- [ ] **Step 1: Write README.md**

```markdown
# skyverse-save-web

A standalone web UI for browsing and editing Skyverse `.sav` files,
built on top of the `gvas` library from the sibling
[`skyverse-save-tool`](../skyverse-save-tool) project. See
`docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md` for the
design.

## Requirements

This module depends on `skyversesave` via a local `replace` directive
pointing at `../skyverse-save-tool` — both projects must be checked out
as siblings on disk.

## Build and run

    go build ./...
    ./skyverseweb -addr :8080

Then open http://localhost:8080 in a browser, upload a `.sav` file,
browse the property tree (click a node to expand it), edit a scalar
value inline and press Save, then click "Download edited save" to get
the result.

Everything (including the vendored htmx script) is embedded in the
binary — no internet access is required at runtime.

## Testing

    go test ./...

Uses the same three real save files as `skyverse-save-tool`'s test
suite (copied into `testdata/`), including a full upload → browse →
edit → download round trip per file.
```

- [ ] **Step 2: Run the full test suite**

Run: `go test ./... -v 2>&1 | tail -60`

Expected: every test PASSes.

- [ ] **Step 3: Run go vet and gofmt**

Run: `go vet ./... && gofmt -l .`

Expected: `go vet` prints nothing; `gofmt -l .` prints nothing (no unformatted files).

- [ ] **Step 4: Full manual smoke test in a real browser flow, simulated via curl**

```bash
go build ./...
./skyverseweb -addr :18080 &
sleep 0.3

# Upload
SESSION_HTML=$(curl -s -F "savefile=@testdata/WorldInfo.sav" http://localhost:18080/upload)
echo "$SESSION_HTML" | grep -o 'WorldName'
SESSION_ID=$(echo "$SESSION_HTML" | grep -oP '(?<=/session/)[a-f0-9]+(?=/download)' | head -1)
echo "session: $SESSION_ID"

# Browse a nested path
curl -s "http://localhost:18080/session/$SESSION_ID/children?path=UDSData.CurrentTime" | grep -o 'Month'

# Edit
curl -s -X POST -d "path=WorldName&value=SmokeTestWorld" "http://localhost:18080/session/$SESSION_ID/edit" | grep -o 'SmokeTestWorld'

# Download and verify
curl -s "http://localhost:18080/session/$SESSION_ID/download" -o /tmp/edited.sav
ls -la /tmp/edited.sav

kill %1
```

Expected: `WorldName` and `Month` and `SmokeTestWorld` each print once; `/tmp/edited.sav` exists and is non-empty.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: add README"
```
