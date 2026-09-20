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
//
// mu guards File itself: SessionStore's own mutex only protects the id ->
// *Session map, not what a handler does with a *Session once it has one.
// Handlers that mutate File (handleEdit) must hold mu for writing; handlers
// that only read it (handleChildren, handleDownload) must hold it for
// reading — see handlers.go.
type Session struct {
	File *gvas.File
	// Filename is the original uploaded file's name (e.g. "Player_Local.sav"),
	// used to name both the /download and /meta responses so they pair up
	// correctly (matching filenames is exactly what the game's own
	// "<file>.sav" + "<file>.sav.meta" sidecar convention requires) without
	// the user needing to rename anything by hand. Immutable after Create,
	// so reading it needs no lock.
	Filename string
	mu       sync.RWMutex
}

// SessionStore is a concurrency-safe in-memory map of session id -> Session.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*Session)}
}

// Create stores f (and the uploaded filename it came from) under a
// freshly minted random id and returns that id.
func (s *SessionStore) Create(f *gvas.File, filename string) (string, error) {
	id, err := newSessionID()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = &Session{File: f, Filename: filename}
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
