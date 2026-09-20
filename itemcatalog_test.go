package main

import "testing"

func TestLoadItemCatalogThreeFieldFormat(t *testing.T) {
	data := []byte(`[["2553_IDA_RepairKit","Resources_2500-2999","/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"]]`)
	catalog, err := loadItemCatalog(data)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	entry, ok := catalog["/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"]
	if !ok {
		t.Fatal("expected a catalog entry for RepairKit's object path")
	}
	if entry.Name != "2553_IDA_RepairKit" {
		t.Errorf("Name = %q, want %q", entry.Name, "2553_IDA_RepairKit")
	}
	if entry.IconFile != "" {
		t.Errorf("IconFile = %q, want empty (no 4th field present in this format)", entry.IconFile)
	}
}

func TestLoadItemCatalogFourFieldFormat(t *testing.T) {
	data := []byte(`[["2553_IDA_RepairKit","Resources_2500-2999","/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit","2553_IDA_RepairKit.png"]]`)
	catalog, err := loadItemCatalog(data)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	entry := catalog["/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"]
	if entry.IconFile != "2553_IDA_RepairKit.png" {
		t.Errorf("IconFile = %q, want %q", entry.IconFile, "2553_IDA_RepairKit.png")
	}
}

func TestLoadItemCatalogSkipsMalformedEntries(t *testing.T) {
	data := []byte(`[["OnlyOneField"],["Name","Category","/Game/Path.Path"]]`)
	catalog, err := loadItemCatalog(data)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("len(catalog) = %d, want 1 (the malformed entry should be skipped)", len(catalog))
	}
}

func TestLoadItemCatalogRejectsInvalidJSON(t *testing.T) {
	if _, err := loadItemCatalog([]byte(`not json`)); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}
