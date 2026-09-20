package main

import (
	"net/http"
	"net/http/httptest"
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
