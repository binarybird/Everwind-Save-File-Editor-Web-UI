package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"skyversesave/gvas"
)

func TestHandleMetaMatchesDownload(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	downloadReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	downloadReq.SetPathValue("id", id)
	downloadRec := httptest.NewRecorder()
	s.handleDownload(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", downloadRec.Code)
	}

	metaReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/meta", nil)
	metaReq.SetPathValue("id", id)
	metaRec := httptest.NewRecorder()
	s.handleMeta(metaRec, metaReq)
	if metaRec.Code != http.StatusOK {
		t.Fatalf("meta status = %d, want 200", metaRec.Code)
	}

	want := gvas.MetaChecksum(downloadRec.Body.Bytes())
	got := metaRec.Body.String()
	if got != want {
		t.Errorf("meta body = %q, want %q (matching the downloaded save's own checksum)", got, want)
	}
	// The .meta content must be a bare decimal integer -- no padding,
	// sign, or trailing newline -- to match the game's own sidecar
	// format exactly.
	if _, err := strconv.ParseUint(got, 10, 32); err != nil {
		t.Errorf("meta body %q is not a plain unsigned decimal: %v", got, err)
	}
}

func TestHandleMetaAndDownloadFilenamesPair(t *testing.T) {
	s := newTestServer(t)
	id := uploadAndGetSessionID(t, s, "testdata/Player_Local.sav")

	downloadReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
	downloadReq.SetPathValue("id", id)
	downloadRec := httptest.NewRecorder()
	s.handleDownload(downloadRec, downloadReq)
	downloadDisposition := downloadRec.Header().Get("Content-Disposition")

	metaReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/meta", nil)
	metaReq.SetPathValue("id", id)
	metaRec := httptest.NewRecorder()
	s.handleMeta(metaRec, metaReq)
	metaDisposition := metaRec.Header().Get("Content-Disposition")

	if !strings.Contains(downloadDisposition, `filename="Player_Local.sav"`) {
		t.Errorf("download Content-Disposition = %q, want it to name Player_Local.sav", downloadDisposition)
	}
	if !strings.Contains(metaDisposition, `filename="Player_Local.sav.meta"`) {
		t.Errorf("meta Content-Disposition = %q, want it to name Player_Local.sav.meta", metaDisposition)
	}
}

func TestHandleMetaUnknownSession(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/session/does-not-exist/meta", nil)
	req.SetPathValue("id", "does-not-exist")
	rec := httptest.NewRecorder()

	s.handleMeta(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSanitizeDownloadFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain name", "Player_Local.sav", "Player_Local.sav"},
		{"strips directory components", "../../etc/passwd", "passwd"},
		{"strips embedded quotes", `evil".sav`, "evil.sav"},
		{"strips control characters", "weird\r\nname.sav", "weirdname.sav"},
		{"empty falls back", "", "edited.sav"},
		{"path-only falls back", "/", "edited.sav"},
		{"dot falls back", ".", "edited.sav"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeDownloadFilename(c.in)
			if got != c.want {
				t.Errorf("sanitizeDownloadFilename(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
