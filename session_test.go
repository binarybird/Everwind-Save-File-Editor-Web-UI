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
