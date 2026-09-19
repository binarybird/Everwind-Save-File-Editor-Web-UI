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
