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
