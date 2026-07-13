package operations

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
)

func req(h *Handler, method, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://x"+path, nil)
	rc := &protocol.RequestContext{Method: method, Path: path, Header: r.Header, Query: map[string]string{}, Raw: r}
	rec := httptest.NewRecorder()
	h.Handle(rec, rc)
	return rec
}

func TestOperationsLifeCycle(t *testing.T) {
	h := New()
	parent := "projects/p/locations/us-central1"
	op := h.Register(parent, false, nil, map[string]any{"target": "x"})
	if op.Done {
		t.Fatal("new pending op should not be done")
	}

	rec := req(h, "GET", "/v2/"+op.Name)
	if rec.Code != 200 {
		t.Fatalf("get op = %d", rec.Code)
	}

	rec = req(h, "POST", "/v2/"+op.Name+":wait")
	if rec.Code != 200 {
		t.Fatalf("wait = %d", rec.Code)
	}

	h.Complete(op.Name, map[string]any{"ok": true})
	rec = req(h, "GET", "/v2/"+op.Name)
	var got Operation
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Done || got.Response["ok"] != true {
		t.Fatalf("op not completed: %+v", got)
	}

	rec = req(h, "GET", "/v2/"+parent+"/operations")
	if !strings.Contains(rec.Body.String(), op.Name) {
		t.Errorf("list missing op")
	}
}

func TestOperationNotFound(t *testing.T) {
	h := New()
	rec := req(h, "GET", "/v2/projects/p/locations/us/operations/nope")
	if rec.Code != 404 {
		t.Fatalf("missing op = %d, want 404", rec.Code)
	}
}
