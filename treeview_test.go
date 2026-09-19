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
