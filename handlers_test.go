package main

import (
	"bytes"
	"html/template"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	tmpl, err := template.ParseFS(templatesFS, "templates/*.tmpl")
	if err != nil {
		t.Fatalf("parsing templates: %v", err)
	}
	sub, err := fs.Sub(embeddedStaticFS, "static")
	if err != nil {
		t.Fatalf("static assets: %v", err)
	}
	staticFS = sub
	return NewServer(NewSessionStore(), tmpl)
}

func TestHandleIndex(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Skyverse Save Viewer") {
		t.Error("index page missing expected title text")
	}
	if !strings.Contains(body, `hx-post="/upload"`) {
		t.Error("index page missing the upload form")
	}
}

func TestStaticAssetsServed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty static/style.css response")
	}
}

func TestHandleUploadValidFile(t *testing.T) {
	s := newTestServer(t)
	req := multipartUploadRequest(t, "testdata/WorldInfo.sav")
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "WorldName") {
		t.Error("upload response missing expected top-level property WorldName")
	}
	if !strings.Contains(body, "hx-swap-oob") {
		t.Error("upload response missing the out-of-band download link")
	}
	if !strings.Contains(body, "/download") {
		t.Error("upload response missing a download link")
	}
}

func TestHandleUploadCorruptFile(t *testing.T) {
	s := newTestServer(t)
	req := multipartUploadRequestBytes(t, []byte("not a save file"))
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Error("expected an error message in the response body")
	}
}

func TestHandleUploadNoFile(t *testing.T) {
	s := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestHandleUploadTooLarge(t *testing.T) {
	s := newTestServer(t)
	oversized := make([]byte, maxUploadSize+1)
	req := multipartUploadRequestBytes(t, oversized)
	rec := httptest.NewRecorder()

	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func multipartUploadRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return multipartUploadRequestBytes(t, data)
}

func multipartUploadRequestBytes(t *testing.T, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("savefile", "upload.sav")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("writing form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}
