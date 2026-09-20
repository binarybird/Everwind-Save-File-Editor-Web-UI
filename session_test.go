package main

import (
	"testing"

	"github.com/binarybird/Everwind-Save-File-Editor-CLI/gvas"
)

func TestSessionStoreCreateAndGet(t *testing.T) {
	store := NewSessionStore()
	f := &gvas.File{}

	id, err := store.Create(f, "Player_Local.sav")
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
	if got.Filename != "Player_Local.sav" {
		t.Errorf("Get returned Filename = %q, want %q", got.Filename, "Player_Local.sav")
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
		id, err := store.Create(f, "test.sav")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if seen[id] {
			t.Fatalf("duplicate session id generated: %s", id)
		}
		seen[id] = true
	}
}
