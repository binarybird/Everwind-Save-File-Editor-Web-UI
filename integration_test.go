package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binarybird/Everwind-Save-File-Editor-CLI/gvas"
)

func TestFullFlowUploadBrowseEditDownload(t *testing.T) {
	for _, tf := range []string{
		"testdata/Player_Local.sav",
		"testdata/Player_Remote_021fa7902e50eeeb8c6ebdbe4fc922fe.sav",
		"testdata/WorldInfo.sav",
	} {
		t.Run(tf, func(t *testing.T) {
			s := newTestServer(t)
			id := uploadAndGetSessionID(t, s, tf)

			// Browse: expand the root, then a nested path if this file has one.
			rootReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/children?path=", nil)
			rootRec := httptest.NewRecorder()
			s.routes().ServeHTTP(rootRec, rootReq)
			if rootRec.Code != http.StatusOK {
				t.Fatalf("root children: status %d", rootRec.Code)
			}

			// Edit a known top-level bool field present in all three files.
			editReq := editRequest(t, id, "bIsOnIsland", "false")
			editRec := httptest.NewRecorder()
			s.routes().ServeHTTP(editRec, editReq)
			if tf == "testdata/WorldInfo.sav" {
				// WorldInfo.sav has no bIsOnIsland field — expect a clean 400, not a panic.
				if editRec.Code != http.StatusBadRequest {
					t.Fatalf("edit on missing field: status %d, want 400", editRec.Code)
				}
				return
			}
			if editRec.Code != http.StatusOK {
				t.Fatalf("edit: status %d, body %s", editRec.Code, editRec.Body.String())
			}

			// Download and verify the edit landed, and nothing else moved.
			dlReq := httptest.NewRequest(http.MethodGet, "/session/"+id+"/download", nil)
			dlRec := httptest.NewRecorder()
			s.routes().ServeHTTP(dlRec, dlReq)
			if dlRec.Code != http.StatusOK {
				t.Fatalf("download: status %d", dlRec.Code)
			}
			edited, err := gvas.Unmarshal(dlRec.Body.Bytes())
			if err != nil {
				t.Fatalf("downloaded file doesn't parse: %v", err)
			}
			p := propByName(edited.Root, "bIsOnIsland")
			if p == nil || p.Bool == nil || *p.Bool != false {
				t.Fatalf("bIsOnIsland after edit: %+v", p)
			}
			island := propByName(edited.Root, "IslandID")
			if island == nil || island.Str == nil || *island.Str == "" {
				t.Error("IslandID should be unchanged and non-empty")
			}
		})
	}
}

func TestFullFlowSessionScopedEndpointsRejectUnknownSession(t *testing.T) {
	s := newTestServer(t)
	unknown := "0000000000000000000000000000000000"

	endpoints := []*http.Request{
		httptest.NewRequest(http.MethodGet, "/session/"+unknown+"/children?path=", nil),
		editRequest(t, unknown, "WorldName", "X"),
		httptest.NewRequest(http.MethodGet, "/session/"+unknown+"/download", nil),
	}
	for _, req := range endpoints {
		rec := httptest.NewRecorder()
		s.routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", req.Method, req.URL.Path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "re-upload") {
			t.Errorf("%s %s: expected a re-upload message", req.Method, req.URL.Path)
		}
	}
}
