package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"skyversesave/gvas"
)

func TestHandleInventoryRendersGrid(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/inventory", nil)
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleInventory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="inventory-grid"`) {
		t.Errorf("expected the inventory grid container, got:\n%s", body)
	}
	if !strings.Contains(body, "/static/icons/") {
		t.Errorf("expected at least one icon <img src=\"/static/icons/...\">, got:\n%s", body)
	}
}

func TestHandleInventoryUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/inventory", nil)
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()

	s.handleInventory(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleEditFromInventoryTabRerendersSlot(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	sess, _ := s.store.Get(id)
	slotPath := "" // filled in below once we know the real path
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath = view.Hotbar[0].SlotPath
	itemPath := view.Hotbar[0].ItemPath

	form := url.Values{}
	form.Set("path", itemPath)
	form.Set("value", "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit_EDITED")
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleEdit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="slot`) {
		t.Errorf("expected a re-rendered <div class=\"slot...\">, got a leafRow instead:\n%s", body)
	}
	if strings.Contains(body, `<li class="leaf"`) {
		t.Error("got a leafRow fragment; slotPath should have routed this to the slot template instead")
	}
}

func TestHandleEditFromTreeTabStillRendersLeafRow(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	form := url.Values{}
	form.Set("path", "IslandID")
	form.Set("value", "changed-value")
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleEdit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<li class="leaf"`) {
		t.Errorf("expected the existing leafRow fragment (no slotPath was sent), got:\n%s", rec.Body.String())
	}
}

func TestHandleSlotRemove(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Hotbar[0].SlotPath // occupied (RepairKit) in testdata

	form := url.Values{}
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotRemove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "/static/icons/") {
		t.Errorf("expected the slot to render empty after removal, got:\n%s", rec.Body.String())
	}

	view2, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView after remove: %v", err)
	}
	if view2.Hotbar[0].Occupied {
		t.Error("Hotbar[0] still Occupied=true after remove")
	}
}

func TestHandleSlotRemoveAlreadyEmpty(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Backpack[53].SlotPath // empty in testdata

	form := url.Values{}
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotRemove(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (slot already empty)", rec.Code)
	}
}

func TestHandleSlotAdd(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Backpack[53].SlotPath // empty in testdata
	itemPath := "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", itemPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotAdd(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	view2, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView after add: %v", err)
	}
	if !view2.Backpack[53].Occupied {
		t.Fatal("Backpack[53] still Occupied=false after add")
	}
	if view2.Backpack[53].ObjectPath != itemPath {
		t.Errorf("Backpack[53].ObjectPath = %q, want %q", view2.Backpack[53].ObjectPath, itemPath)
	}
}

func TestHandleSlotAddUnknownItem(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Backpack[53].SlotPath

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", "/Game/Not/A/Real/Item.Item")
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotAdd(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (unknown item)", rec.Code)
	}
}

// TestHandleSlotAddRoundTripsThroughDownload exercises the spec's
// required end-to-end path: add an item, download, re-parse with
// gvas.Unmarshal, and confirm the new item -- built via buildNewItem's
// real clone-or-template logic, not a hand-built one-field stub -- is
// really there with the right BaseData and quantity. This is the
// regression net that would have caught SlotsWithItems being left at 0
// after an add.
func TestHandleSlotAddRoundTripsThroughDownload(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Backpack[53].SlotPath // empty in testdata
	itemPath := "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit"

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", itemPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.handleSlotAdd(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("slot/add status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	downloadReq.SetPathValue("id", id)
	downloadRec := httptest.NewRecorder()
	s.handleDownload(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", downloadRec.Code)
	}

	f2, err := gvas.Unmarshal(downloadRec.Body.Bytes())
	if err != nil {
		t.Fatalf("re-Unmarshal downloaded save: %v", err)
	}
	itemsProp, err := gvas.Lookup(f2, slotPath+".Items")
	if err != nil {
		t.Fatalf("re-parsed save: resolving %s.Items: %v", slotPath, err)
	}
	if itemsProp.Array == nil || len(itemsProp.Array.Structs) != 1 {
		t.Fatalf("re-parsed save: %s.Items has %d elements, want 1", slotPath, arrayLen(itemsProp.Array))
	}
	base := findFieldProp(itemsProp.Array.Structs[0], "BaseData")
	if base == nil || base.Str == nil || *base.Str != itemPath {
		t.Errorf("re-parsed save: added item's BaseData = %+v, want %q", base, itemPath)
	}
	qtyProp, err := gvas.Lookup(f2, slotPath+".SlotsWithItems")
	if err != nil || qtyProp.Int32 == nil || *qtyProp.Int32 != 1 {
		t.Errorf("re-parsed save: %s.SlotsWithItems = %v, want 1", slotPath, qtyProp)
	}
}

// TestHandleSlotRemoveRoundTripsThroughDownload exercises the spec's
// required end-to-end path: remove an item, download, re-parse, confirm
// the slot's Items is empty and SlotsWithItems is reset, and confirm an
// unrelated top-level field is untouched.
func TestHandleSlotRemoveRoundTripsThroughDownload(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Hotbar[0].SlotPath // occupied (RepairKit) in testdata

	form := url.Values{}
	form.Set("slotPath", slotPath)
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	s.handleSlotRemove(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("slot/remove status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	downloadReq.SetPathValue("id", id)
	downloadRec := httptest.NewRecorder()
	s.handleDownload(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", downloadRec.Code)
	}

	f2, err := gvas.Unmarshal(downloadRec.Body.Bytes())
	if err != nil {
		t.Fatalf("re-Unmarshal downloaded save: %v", err)
	}
	itemsProp, err := gvas.Lookup(f2, slotPath+".Items")
	if err != nil {
		t.Fatalf("re-parsed save: resolving %s.Items: %v", slotPath, err)
	}
	if itemsProp.Array == nil || len(itemsProp.Array.Structs) != 0 {
		t.Fatalf("re-parsed save: %s.Items has %d elements, want 0", slotPath, arrayLen(itemsProp.Array))
	}
	qtyProp, err := gvas.Lookup(f2, slotPath+".SlotsWithItems")
	if err != nil || qtyProp.Int32 == nil || *qtyProp.Int32 != 0 {
		t.Errorf("re-parsed save: %s.SlotsWithItems = %v, want 0", slotPath, qtyProp)
	}

	original, err := gvas.Unmarshal(readTestdata(t, "Player_Local.sav"))
	if err != nil {
		t.Fatalf("Unmarshal original testdata: %v", err)
	}
	island := findFieldProp(f2.Root, "IslandID")
	wantIsland := findFieldProp(original.Root, "IslandID")
	if island == nil || wantIsland == nil || *island.Str != *wantIsland.Str {
		t.Errorf("IslandID changed unexpectedly: got %+v, want %+v", island, wantIsland)
	}
}

func TestHandleSlotAddAlreadyOccupied(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	sess, _ := s.store.Get(id)
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	if err != nil {
		t.Fatalf("BuildInventoryGridView: %v", err)
	}
	slotPath := view.Hotbar[0].SlotPath // occupied

	form := url.Values{}
	form.Set("slotPath", slotPath)
	form.Set("itemPath", "/Game/Data/Items/Resources_2500-2999/2553_IDA_RepairKit.2553_IDA_RepairKit")
	req := httptest.NewRequest(http.MethodPost, "/session/"+id+"/slot/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	s.handleSlotAdd(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (slot already occupied)", rec.Code)
	}
}
