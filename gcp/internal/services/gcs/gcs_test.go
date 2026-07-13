package gcs

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
)

func route(h *Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://storage.googleapis.com"+path, strings.NewReader(body))
	rc := &protocol.RequestContext{
		Method: method, Path: path, Header: r.Header,
		Query: map[string]string{}, Body: io.NopCloser(strings.NewReader(body)), Raw: r,
	}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			rc.Query[k] = v[0]
		}
	}
	rec := httptest.NewRecorder()
	h.Handle(rec, rc)
	return rec
}

func TestBucketLifecycle(t *testing.T) {
	h := New()

	rec := route(h, "POST", "/storage/v1/b?project=p1", `{"name": "my-bucket", "location": "us-central1", "storageClass":"NEARLINE"}`)
	if rec.Code != 200 {
		t.Fatalf("create bucket = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var b Bucket
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if b.Kind != "storage#bucket" || b.Name != "my-bucket" {
		t.Fatalf("unexpected bucket: %+v", b)
	}
	if b.Location != "US-CENTRAL1" || b.StorageClass != "NEARLINE" {
		t.Errorf("location/class not honored: %s / %s", b.Location, b.StorageClass)
	}

	rec = route(h, "POST", "/storage/v1/b?project=p1", `{"name": "my-bucket"}`)
	if rec.Code != 409 {
		t.Fatalf("duplicate create = %d, want 409", rec.Code)
	}

	rec = route(h, "GET", "/storage/v1/b/my-bucket", "")
	if rec.Code != 200 {
		t.Fatalf("get bucket = %d, want 200", rec.Code)
	}

	rec = route(h, "GET", "/storage/v1/b?project=p1", "")
	var list map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if items, ok := list["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("list expected 1 bucket, got %v", list["items"])
	}

	rec = route(h, "PATCH", "/storage/v1/b/my-bucket", `{"storageClass" : "COLDLINE" }`)
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if b.StorageClass != "COLDLINE" || b.Metageneration != "2" {
		t.Fatalf("patch not applied: class=%s metagen=%s", b.StorageClass, b.Metageneration)
	}

	rec = route(h, "DELETE", "/storage/v1/b/my-bucket", "")
	if rec.Code != 204 {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}
	rec = route(h, "GET", "/storage/v1/b/my-bucket", "")
	if rec.Code != 404 {
		t.Fatalf("get after delete = %d, want 204", rec.Code)
	}
}

func TestBucketNameRequired(t *testing.T)  {
	h := New()
	rec := route(h, "POST", "/storage/v1/b", `{}`)
	if rec.Code != 400 {
		t.Fatalf("missing name = %d, want 400", rec.Code)
	}
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	errObj, _ := env["error"].(map[string]any)
	if errObj["status"] != "INVALID_ARGUMENT" {
		t.Errorf("error status = %v, want INVALID_ARGUMENT", errObj["status"])
	}
}

func TestObjectLifeCycle(t *testing.T)  {
	h := New()
	route(h, "POST", "/storage/v1/b?project=p1", `{"name": "b1"}`)

	rc := &protocol.RequestContext{Header: map[string][]string{"Host": {"storage.googleapis.com"}}}
	obj, gcpErr := h.InsertObject(rc, "b1", "hello.txt", "text/plain", []byte("hello world"))
	if gcpErr != nil {
		t.Fatalf("insert object failed: %v", gcpErr)
	}
	if obj.Size != "11" || obj.Md5Hash == "" || obj.Crc32c == "" {
		t.Errorf("object metadata wrong: size=%s md5=%s crc=%s", obj.Size, obj.Md5Hash, obj.Crc32c)
	}

	rec := route(h, "GET", "/storage/v1/b/b1/o", "")
	var list map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if items, ok := list["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("list objects expected 1, got %v", list["items"])
	}

	rec = route(h, "GET", "/storage/v1/b/b1/o/hello.txt", "")
	if rec.Code != 200 {
		t.Fatalf("get object = %d, want 200", rec.Code)
	}

	rec = route(h, "GET", "/storage/v1/b/b1/o/hello.txt?alt=media", "")
	if rec.Code != 200 || rec.Body.String() != "hello world" {
		t.Fatalf("download = %d, body = %q", rec.Code, rec.Body.String())
	}

	rec = route(h, "DELETE", "/storage/v1/b/b1/o/hello.txt", "")
	if rec.Code != 204 {
		t.Fatalf("delete object = %d, want 204", rec.Code)
	}
}

func TestObjectBucketNotFound(t *testing.T)  {
	h := New()
	rec := route(h, "GET", "/storage/v1/b/nope/o", "")
	if rec.Code != 404 {
		t.Fatalf("object list on missing bucket = %d, want 404", rec.Code)
	}
}

func TestClaimsPath(t *testing.T) {
	h := New()
	if !h.ClaimsPath("/storage/v1/b") || !h.ClaimsPath("/storage/v1/b/x/o/y") {
		t.Error("should claim /storage/v1/b paths")
	}
	if h.ClaimsPath("/v1/projects/p/topics") {
		t.Error("should not claim pubsub paths")
	}
}