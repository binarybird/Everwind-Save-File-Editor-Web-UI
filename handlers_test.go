package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"skyversesave/gvas"
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
	itemsData, err := embeddedStaticFS.ReadFile("static/items.json")
	if err != nil {
		t.Fatalf("reading items.json: %v", err)
	}
	catalog, err := loadItemCatalog(itemsData)
	if err != nil {
		t.Fatalf("loadItemCatalog: %v", err)
	}
	return NewServer(NewSessionStore(), tmpl, catalog)
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

func uploadAndGetSessionID(t *testing.T, s *Server, testdataFile string) string {
	t.Helper()
	req := multipartUploadRequest(t, testdataFile)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed: status %d, body %s", rec.Code, rec.Body.String())
	}
	// Extract the session id from the download link href="/session/<id>/download".
	body := rec.Body.String()
	const marker = `/session/`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("could not find session id in upload response: %s", body)
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "/")
	if j < 0 {
		t.Fatalf("could not parse session id from: %s", rest)
	}
	return rest[:j]
}

func TestHandleChildrenNestedPath(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=UDSData.CurrentTime", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Month", "Day", "TimeOfDay"} {
		if !strings.Contains(body, want) {
			t.Errorf("children fragment missing expected field %q", want)
		}
	}
}

func TestHandleChildrenPagination(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=PlayerMarkerSettings.Filters&limit=3", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Load more") {
		t.Error("expected a 'Load more' control when more items remain")
	}
}

// TestHandleChildrenStructArrayElement is the handler-level regression test
// for the finding that GET /session/{id}/children?path=Components[0]
// returned 400 (gvas.Lookup refuses to resolve a path that ends bare on an
// array index) even though the corresponding path=Components listing (each
// item Expandable with Path "Components[0]" etc.) works fine.
func TestHandleChildrenStructArrayElement(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=Components[0]", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ComponentName") {
		t.Error("children fragment missing expected field ComponentName")
	}
}

func TestHandleChildrenUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/children?path=", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "re-upload") {
		t.Error("expected a 'please re-upload' message for an unknown session")
	}
}

func multipartUploadRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return multipartUploadRequestBytesNamed(t, data, filepath.Base(path))
}

func multipartUploadRequestBytes(t *testing.T, data []byte) *http.Request {
	t.Helper()
	return multipartUploadRequestBytesNamed(t, data, "upload.sav")
}

func multipartUploadRequestBytesNamed(t *testing.T, data []byte, filename string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("savefile", filename)
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

func TestHandleEditString(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := editRequest(t, id, "WorldName", "NewName")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NewName") {
		t.Error("edit response should show the new value")
	}

	sess, _ := s.store.Get(id)
	p, err := gvas.Lookup(sess.File, "WorldName")
	if err != nil || p.Str == nil || *p.Str != "NewName" {
		t.Fatalf("session tree not updated: %+v, err=%v", p, err)
	}
}

// TestHandleEditObjectPropertyPath confirms an ObjectProperty value (e.g.
// an inventory slot's BaseData item-asset reference) is editable through
// the web UI. This is cross-project behavior: gvas decodes ObjectProperty
// as a plain string when possible, and this handler never special-cases
// property types — it just works because everything here dispatches on
// Property.Str, not Type. See skyversesave/gvas's decode.go.
func TestHandleEditObjectPropertyPath(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	path := "Components[1].Data.Data.Slots[0].Items[0].BaseData"
	newValue := "/Game/Data/Items/Resources_2500-2999/9999_IDA_TestSwap.9999_IDA_TestSwap"

	req := editRequest(t, id, path, newValue)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), newValue) {
		t.Error("edit response should show the new object path")
	}

	sess, _ := s.store.Get(id)
	p, err := gvas.Lookup(sess.File, path)
	if err != nil || p.Str == nil || *p.Str != newValue {
		t.Fatalf("session tree not updated: %+v, err=%v", p, err)
	}
}

// TestHandleChildrenObjectPropertyGetsItemPicker confirms only
// ObjectProperty fields (e.g. BaseData) render the searchable item-picker
// markup — a plain StrProperty like WorldName must render an ordinary
// text input, not the picker wrapper, so the picker's JS doesn't attach
// to fields it can't sensibly autocomplete.
func TestHandleChildrenObjectPropertyGetsItemPicker(t *testing.T) {
	s := newTestServer(t)

	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")
	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=Components[1].Data.Data.Slots[0].Items[0]", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="item-picker"`) {
		t.Error("BaseData (ObjectProperty) should render the item-picker wrapper")
	}
	if !strings.Contains(body, `data-item-picker="1"`) {
		t.Error("BaseData (ObjectProperty) input should carry data-item-picker")
	}

	id2 := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")
	rootReq := httptest.NewRequest(http.MethodGet, "/session/"+id2+"/children?path=", nil)
	rootRec := httptest.NewRecorder()
	s.routes().ServeHTTP(rootRec, rootReq)
	rootBody := rootRec.Body.String()
	if strings.Contains(rootBody, `data-item-picker`) {
		t.Error("a plain StrProperty (WorldName) should not get the item-picker wrapper")
	}
}

func TestHandleEditBadValue(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := editRequest(t, id, "bNewGame", "not-a-bool")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Error("expected an inline error message")
	}

	sess, _ := s.store.Get(id)
	p, err := gvas.Lookup(sess.File, "bNewGame")
	if err != nil || p.Bool == nil || *p.Bool != false {
		t.Fatalf("session tree should be unchanged after a failed edit: %+v, err=%v", p, err)
	}
}

func TestHandleEditNonScalarRejected(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := editRequest(t, id, "UDSData", "whatever")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleEditUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := editRequest(t, "does-not-exist", "WorldName", "X")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func editRequest(t *testing.T, sessionID, path, value string) *http.Request {
	t.Helper()
	form := url.Values{"path": {path}, "value": {value}}
	req := httptest.NewRequest(http.MethodPost, "/session/"+sessionID+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestHandleDownload(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", ct)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Error("expected Content-Disposition: attachment")
	}

	f, err := gvas.Unmarshal(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("downloaded bytes don't parse as a save file: %v", err)
	}
	if len(f.Root) == 0 {
		t.Error("downloaded file has no top-level properties")
	}
}

func TestHandleDownloadReflectsEdit(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	editReq := editRequest(t, id, "WorldName", "EditedViaWeb")
	editRec := httptest.NewRecorder()
	s.routes().ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("edit failed: status %d, body %s", editRec.Code, editRec.Body.String())
	}

	original, err := os.ReadFile("testdata/WorldInfo.sav")
	if err != nil {
		t.Fatal(err)
	}
	originalFile, err := gvas.Unmarshal(original)
	if err != nil {
		t.Fatal(err)
	}

	dlReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	dlRec := httptest.NewRecorder()
	s.routes().ServeHTTP(dlRec, dlReq)

	edited, err := gvas.Unmarshal(dlRec.Body.Bytes())
	if err != nil {
		t.Fatalf("downloaded bytes don't parse: %v", err)
	}
	name := propByName(edited.Root, "WorldName")
	if name == nil || *name.Str != "EditedViaWeb" {
		t.Fatalf("WorldName after edit+download: %+v", name)
	}
	version := propByName(edited.Root, "GameVersion")
	wantVersion := propByName(originalFile.Root, "GameVersion")
	if version == nil || wantVersion == nil || *version.Str != *wantVersion.Str {
		t.Errorf("GameVersion changed unexpectedly: got %+v, want %+v", version, wantVersion)
	}
}

func propByName(props []*gvas.Property, name string) *gvas.Property {
	for _, p := range props {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// TestConcurrentEditAndDownloadNoDataRace is the regression test for the
// finding that a session's *gvas.File tree has no synchronization once a
// handler obtains it via SessionStore.Get: handleEdit's writes (via
// gvas.Property.Set*) race with handleDownload's reads (via gvas.Marshal
// walking the same tree) and handleChildren's reads (via childrenOf), all
// against the SAME session id. Run with `go test -race` — that's what
// surfaces the race; a plain `go test` run won't flag it.
func TestConcurrentEditAndDownloadNoDataRace(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/WorldInfo.sav")

	const goroutines = 6
	const itersEach = 3 // ~18 total ops across edit/download/children, kept modest for speed

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < itersEach; i++ {
				switch g % 3 {
				case 0:
					req := editRequest(t, id, "WorldName", fmt.Sprintf("race-%d-%d", g, i))
					rec := httptest.NewRecorder()
					s.routes().ServeHTTP(rec, req)
				case 1:
					req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
					rec := httptest.NewRecorder()
					s.routes().ServeHTTP(rec, req)
				default:
					req := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=", nil)
					rec := httptest.NewRecorder()
					s.routes().ServeHTTP(rec, req)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestHandleDownloadUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/download", nil)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
