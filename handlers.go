package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"

	"skyversesave/gvas"
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
	mux.HandleFunc("POST /upload", s.handleUpload)
	mux.HandleFunc("GET /session/{id}/children", s.handleChildren)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if err := s.templates.ExecuteTemplate(w, "index.html.tmpl", nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

const maxUploadSize = 50 << 20 // 50MB, generously above the ~1MB real samples

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("file too large or malformed upload: %w", err))
		return
	}
	file, _, err := r.FormFile("savefile")
	if err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("no file provided: %w", err))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("reading upload: %w", err))
		return
	}

	parsed, err := gvas.Unmarshal(data)
	if err != nil {
		renderError(w, http.StatusUnprocessableEntity, fmt.Errorf("this doesn't look like a valid save file: %w", err))
		return
	}

	id, err := s.store.Create(parsed)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("creating session: %w", err))
		return
	}

	items, _, err := childrenOf(parsed, "", 0, 0)
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("rendering root: %w", err))
		return
	}
	view := ChildrenView{SessionID: id, Items: toItemViews(id, items)}
	if err := s.templates.ExecuteTemplate(w, "uploadResponse", view); err != nil {
		log.Printf("rendering uploadResponse: %v", err)
	}
}

func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	path := r.URL.Query().Get("path")
	offset := queryInt(r, "offset", 0)
	limit := queryInt(r, "limit", 0)

	items, hasMore, err := childrenOf(sess.File, path, offset, limit)
	if err != nil {
		renderError(w, http.StatusBadRequest, err)
		return
	}

	view := ChildrenView{SessionID: id, Items: toItemViews(id, items)}
	if hasMore {
		nextOffset := offset + limit
		if limit <= 0 {
			nextOffset = offset + defaultChildrenLimit
		}
		view.HasMore = true
		view.LoadMoreURL = fmt.Sprintf("/session/%s/children?path=%s&offset=%d&limit=%d", id, path, nextOffset, limitOrDefault(limit))
	}

	if err := s.templates.ExecuteTemplate(w, "children", view); err != nil {
		log.Printf("rendering children: %v", err)
	}
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func limitOrDefault(limit int) int {
	if limit <= 0 {
		return defaultChildrenLimit
	}
	return limit
}

func renderSessionNotFound(w http.ResponseWriter) {
	renderError(w, http.StatusNotFound, fmt.Errorf("session not found or expired — please re-upload your save file"))
}

// renderError writes a small inline error fragment. Used by every handler
// for its failure paths — see docs/superpowers/specs/2026-09-19-skyverse-save-web-design.md,
// "Error handling".
func renderError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	fmt.Fprintf(w, `<div class="error">%s</div>`, template.HTMLEscapeString(err.Error()))
}
