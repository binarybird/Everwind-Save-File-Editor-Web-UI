package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/binarybird/Everwind-Save-File-Editor-CLI/gvas"
)

// Server holds the shared dependencies every handler needs.
type Server struct {
	store       *SessionStore
	templates   *template.Template
	itemCatalog map[string]catalogEntry
}

func NewServer(store *SessionStore, templates *template.Template, itemCatalog map[string]catalogEntry) *Server {
	return &Server{store: store, templates: templates, itemCatalog: itemCatalog}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("POST /upload", s.handleUpload)
	mux.HandleFunc("GET /session/{id}/children", s.handleChildren)
	mux.HandleFunc("POST /session/{id}/edit", s.handleEdit)
	mux.HandleFunc("GET /session/{id}/download", s.handleDownload)
	mux.HandleFunc("GET /session/{id}/meta", s.handleMeta)
	mux.HandleFunc("GET /session/{id}/inventory", s.handleInventory)
	mux.HandleFunc("POST /session/{id}/slot/add", s.handleSlotAdd)
	mux.HandleFunc("POST /session/{id}/slot/remove", s.handleSlotRemove)
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
	file, header, err := r.FormFile("savefile")
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

	id, err := s.store.Create(parsed, sanitizeDownloadFilename(header.Filename))
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

	sess.mu.RLock()
	items, hasMore, err := childrenOf(sess.File, path, offset, limit)
	sess.mu.RUnlock() // release before rendering — only the tree read needs the lock
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

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	path := r.FormValue("path")
	value := r.FormValue("value")
	slotPath := r.FormValue("slotPath") // set only by Inventory-tab edit forms; "" for Tree-tab edits

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Validate slotPath resolves BEFORE touching path, so a tampered
	// request carrying a slotPath that doesn't match path can never leave
	// the session half-edited (path's edit applied, then a failure
	// re-rendering slotPath).
	if slotPath != "" {
		if _, isContainer, err := propsAt(sess.File, slotPath); err != nil || !isContainer {
			renderError(w, http.StatusBadRequest, fmt.Errorf("invalid slot %q", slotPath))
			return
		}
	}

	p, err := gvas.Lookup(sess.File, path)
	if err != nil {
		if slotPath != "" {
			s.renderSlot(w, sess.File, id, slotPath, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		s.templates.ExecuteTemplate(w, "leafRow", itemView{
			childItem: childItem{Label: lastPathSegment(path), Path: path, Editable: true, Preview: value},
			SessionID: id,
			RowID:     rowID(path),
			Error:     err.Error(),
		})
		return
	}

	preview, editErr := applyEdit(p, value)
	status := http.StatusOK
	errMsg := ""
	if editErr != nil {
		status = http.StatusBadRequest
		errMsg = editErr.Error()
		preview = previewOf(p) // unchanged — applyEdit never partially mutates on a parse error
	}

	if slotPath != "" {
		s.renderSlot(w, sess.File, id, slotPath, status, errMsg)
		return
	}

	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "leafRow", itemView{
		childItem: childItem{Label: lastPathSegment(path), Type: p.Type, Path: path, Editable: true, Preview: preview},
		SessionID: id,
		RowID:     rowID(path),
		Error:     errMsg,
	})
}

// renderSlot re-renders one Inventory-tab slot from f's current state,
// used after an edit, add, or remove targeting that slot. status/errMsg
// let a failed operation still re-render the slot (with its unchanged
// data) alongside an inline error, matching the Tree tab's leafRow
// error-handling pattern.
func (s *Server) renderSlot(w http.ResponseWriter, f *gvas.File, sessionID, slotPath string, status int, errMsg string) {
	// slotPath always ends bare on a struct-array index (e.g.
	// "Components[1].Data.Data.Slots[0]"), which gvas.Lookup intentionally
	// refuses to resolve directly (see treeview.go's propsAt/splitTrailingIndex
	// docs) — so resolve it the same way the Tree tab does.
	props, isContainer, err := propsAt(f, slotPath)
	if err != nil {
		// A client-supplied slotPath that doesn't resolve is bad input, not
		// a server fault.
		renderError(w, http.StatusBadRequest, fmt.Errorf("re-rendering slot %q: %w", slotPath, err))
		return
	}
	if !isContainer {
		renderError(w, http.StatusBadRequest, fmt.Errorf("re-rendering slot %q: not a container", slotPath))
		return
	}
	sv := buildSlotView(props, slotPath, sessionID, s.itemCatalog)
	sv.Error = errMsg
	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "slot", sv)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	sess.mu.RLock()
	data, err := gvas.Marshal(sess.File)
	sess.mu.RUnlock()
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("encoding save: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sess.Filename))
	w.Write(data)
}

// handleMeta returns the session's current save data's ".meta" sidecar
// content: the exact plain-text CRC32 checksum the game compares
// against on load (see gvas.MetaChecksum) -- a save written by anything
// other than the game itself needs this sidecar rewritten to match, or
// the game treats the file as corrupt and silently falls back to its
// own ".backup". Named "<Filename>.meta" so it pairs correctly with the
// /download response's filename in the game's own save directory.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}

	sess.mu.RLock()
	data, err := gvas.Marshal(sess.File)
	sess.mu.RUnlock()
	if err != nil {
		renderError(w, http.StatusInternalServerError, fmt.Errorf("encoding save: %w", err))
		return
	}

	checksum := gvas.MetaChecksum(data)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.meta"`, sess.Filename))
	w.Write([]byte(checksum))
}

// sanitizeDownloadFilename turns an untrusted, user-supplied upload
// filename into one safe to echo back in a Content-Disposition header:
// path components stripped (filepath.Base), double quotes and control
// characters (which could otherwise break out of the header's quoted
// filename value) removed, falling back to "edited.sav" for an empty,
// path-only ("/", ".", ".."), or otherwise-empty-after-sanitizing name.
func sanitizeDownloadFilename(name string) string {
	const fallback = "edited.sav"
	name = filepath.Base(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fallback
	}
	name = strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" {
		return fallback
	}
	return name
}

func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}
	sess.mu.RLock()
	view, err := BuildInventoryGridView(sess.File, id, s.itemCatalog)
	sess.mu.RUnlock()
	if err != nil {
		renderError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "inventory", view); err != nil {
		log.Printf("rendering inventory: %v", err)
	}
}

func (s *Server) handleSlotRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	slotPath := r.FormValue("slotPath")

	sess.mu.Lock()
	defer sess.mu.Unlock()

	itemsProp, err := gvas.Lookup(sess.File, slotPath+".Items")
	if err != nil {
		// slotPath itself doesn't resolve at all (only reachable via a
		// tampered request, never through the normal UI) -- there's no
		// slot to re-render, so this is the one case that still falls
		// back to a bare error fragment.
		renderError(w, http.StatusBadRequest, fmt.Errorf("resolving slot: %w", err))
		return
	}
	if itemsProp.Array == nil || len(itemsProp.Array.Structs) == 0 {
		// slotPath resolves fine, so re-render the real slot (with its
		// Error set) rather than a bare error div -- this response goes
		// to the same hx-target="#slot-{rowID}" hx-swap="outerHTML" the
		// success path uses, so it must keep the same "whole slot"
		// shape or it would replace the slot with just an error message.
		s.renderSlot(w, sess.File, id, slotPath, http.StatusBadRequest, "slot is already empty")
		return
	}
	if err := itemsProp.RemoveStructElement(0); err != nil {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusInternalServerError, err.Error())
		return
	}
	if err := setSlotQuantity(sess.File, slotPath, 0); err != nil {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderSlot(w, sess.File, id, slotPath, http.StatusOK, "")
}

func (s *Server) handleSlotAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := s.store.Get(id)
	if !ok {
		renderSessionNotFound(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("bad form: %w", err))
		return
	}
	slotPath := r.FormValue("slotPath")
	itemPath := r.FormValue("itemPath")

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Resolve slotPath first (regardless of itemPath validity) so every
	// failure from here on can re-render the real slot with its Error
	// set, matching the hx-target="#slot-{rowID}" hx-swap="outerHTML"
	// every add form already uses -- a bare error fragment swapped in
	// there would replace the whole slot instead of showing the error
	// alongside it.
	itemsProp, err := gvas.Lookup(sess.File, slotPath+".Items")
	if err != nil {
		renderError(w, http.StatusBadRequest, fmt.Errorf("resolving slot: %w", err))
		return
	}
	if _, ok := s.itemCatalog[itemPath]; !ok {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusBadRequest, fmt.Sprintf("unknown item %q", itemPath))
		return
	}
	if itemsProp.Array == nil {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusInternalServerError, fmt.Sprintf("slot %q has no Items array", slotPath))
		return
	}
	if len(itemsProp.Array.Structs) != 0 {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusBadRequest, "slot already has an item")
		return
	}
	newItem, err := buildNewItem(sess.File, itemPath)
	if err != nil {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusInternalServerError, err.Error())
		return
	}
	if err := itemsProp.AppendStructElement(newItem); err != nil {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusInternalServerError, err.Error())
		return
	}
	if err := setSlotQuantity(sess.File, slotPath, 1); err != nil {
		s.renderSlot(w, sess.File, id, slotPath, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderSlot(w, sess.File, id, slotPath, http.StatusOK, "")
}

// setSlotQuantity sets the SlotsWithItems field of the slot at slotPath,
// clamped to that slot's MaxStackCount if present (never above it).
// handleSlotAdd/handleSlotRemove must keep SlotsWithItems consistent with
// Items themselves -- it's the field the game (and this app's own grid)
// reads as the stack quantity, not merely whether Items is non-empty; a
// prior version of this code left it stale, so e.g. a remove-then-add on
// the same slot silently inherited the removed item's old quantity.
func setSlotQuantity(f *gvas.File, slotPath string, qty int32) error {
	qtyProp, err := gvas.Lookup(f, slotPath+".SlotsWithItems")
	if err != nil {
		return fmt.Errorf("resolving slot quantity: %w", err)
	}
	if maxProp, err := gvas.Lookup(f, slotPath+".MaxStackCount"); err == nil && maxProp.Int32 != nil && qty > *maxProp.Int32 {
		qty = *maxProp.Int32
	}
	return qtyProp.SetInt32(qty)
}

// applyEdit parses value against p's existing scalar kind and applies it,
// mirroring skyverse-save-tool's cmd/saveview/set.go applyScalarEdit. It
// returns the freshly formatted preview string on success.
func applyEdit(p *gvas.Property, value string) (string, error) {
	switch {
	case p.Str != nil:
		if err := p.SetString(value); err != nil {
			return "", err
		}
		return value, nil
	case p.Bool != nil:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("parsing %q as bool: %w", value, err)
		}
		if err := p.SetBool(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%v", v), nil
	case p.Int32 != nil:
		v, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return "", fmt.Errorf("parsing %q as int32: %w", value, err)
		}
		if err := p.SetInt32(int32(v)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case p.Int64 != nil:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return "", fmt.Errorf("parsing %q as int64: %w", value, err)
		}
		if err := p.SetInt64(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", v), nil
	case p.Float32 != nil:
		v, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return "", fmt.Errorf("parsing %q as float32: %w", value, err)
		}
		if err := p.SetFloat32(float32(v)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%g", v), nil
	case p.Float64 != nil:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", fmt.Errorf("parsing %q as float64: %w", value, err)
		}
		if err := p.SetFloat64(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%g", v), nil
	default:
		return "", fmt.Errorf("%q (type %s) is not an editable scalar", p.Name, p.Type)
	}
}

// previewOf formats p's current scalar value, for redisplay after a
// failed edit. Returns "" for a non-scalar property (shouldn't happen —
// callers only use this after applyEdit already confirmed p is scalar-shaped).
func previewOf(p *gvas.Property) string {
	s, _ := scalarPreview(p)
	return s
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
