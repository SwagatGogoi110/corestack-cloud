package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
)

func buildHandler() *protocol.Registry {
	reg := protocol.NewRegistry()
	registerAll(reg)
	return reg
}

func TestHealthEndpoints(t *testing.T) {
	h := topHandler(buildHandler())
	for _, path := range []string{"/health", "/_floci/health"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "http://x"+path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s = %d, want 200", path, rec.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s body not JSON: %v", path, err)
		}
		if body["status"] != "ok" {
			t.Errorf("%s status = %v, want ok", path, body["status"])
		}

		svcs, _ := body["services"].([]any)
		if len(svcs) < 18 {
			t.Errorf("%s reported %d services, want >= 18", path, len(svcs))
		}
	}
}

func TestUnknownPathFailsThroughToRegistry(t *testing.T) {
	h := topHandler(buildHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://x/v1/projects/p/nonexistent", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("unknown API path = %d, want 404 from registry", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("expected GCP error envelope, got: %s", rec.Body.String())
	}
}

func TestKnownPathRoutesThroughMux(t *testing.T) {
	h := topHandler(buildHandler())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://x/storage/v1/b?project=p", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("gcs list-buckets should route through the mux, got 404: %s", rec.Body.String())
	}
}
