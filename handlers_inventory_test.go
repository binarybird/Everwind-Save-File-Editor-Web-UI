package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
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
	slotPath = view.Backpack[0].SlotPath
	itemPath := view.Backpack[0].ItemPath

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
