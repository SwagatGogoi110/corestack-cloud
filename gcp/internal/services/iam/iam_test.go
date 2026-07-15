package iam

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
)

func req(h *Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://iam.googleapis.com"+path, strings.NewReader(body))
	rc := &protocol.RequestContext{
		Method: method,
		Path:   path,
		Header: r.Header,
		Query:  map[string]string{},
		Body:   io.NopCloser(strings.NewReader(body)),
		Raw:    r,
	}
	w := httptest.NewRecorder()
	h.Handle(w, rc)
	return w
}

func TestServiceAccountLifecycle(t *testing.T) {
	h := New()
	base := "/v1/projects/proj/serviceAccounts"

	rec := req(h, "POST", base, `{"accountId":"my-sa", "serviceAccount": {"displayName": "My SA"}}`)
	if rec.Code != 200 {
		t.Fatalf("create SA = %d; %s", rec.Code, rec.Body.String())
	}
	var sa ServiceAccount
	_ = json.Unmarshal(rec.Body.Bytes(), &sa)
	if sa.Email != "my-sa@proj.iam.gserviceaccount.com" || sa.DisplayName != "My SA" {
		t.Fatalf("unexpected SA: %+v", sa)
	}

	// get it back
	rec = req(h, "GET", base+"/"+sa.Email, "")
	if rec.Code != 200 {
		t.Fatalf("get SA = %d", rec.Code)
	}

	// list all
	rec = req(h, "GET", base, "")
	if !strings.Contains(rec.Body.String(), sa.Email) {
		t.Error("list missing SA")
	}

	// create a key
	rec = req(h, "POST", base+"/"+sa.Email+"/keys", "")
	if rec.Code != 200 {
		t.Fatalf("create key = %d", rec.Code)
	}
	var key Key
	_ = json.Unmarshal(rec.Body.Bytes(), &key)
	if key.PrivateKeyData == "" {
		t.Error("create key should return PrivateKeyData")
	}

	// List keys (private data omitted)
	rec = req(h, "GET", base+"/"+sa.Email+"/keys", "")
	if strings.Contains(rec.Body.String(), "privateKeyData") {
		t.Error("list keys should not return privateKeyData")
	}

	// IAM Policy set/get
	rec = req(h, "POST", sa.Name+":setIamPolicy", `{"policy": {"bindings": [{"role": "roles/viewer", "members": ["user:a@b.com"]}]}}`)
	if rec.Code != 200 {
		t.Fatalf("set IAM policy = %d", rec.Code)
	}

	rec = req(h, "POST", base+"/"+sa.Email+":getIamPolicy", "")
	if !strings.Contains(rec.Body.String(), "roles/viewer") {
		t.Error("getIAMPolicy lost the binding")
	}

	// delete
	rec = req(h, "DELETE", base+"/"+sa.Email, "")
	if rec.Code != 200 {
		t.Fatalf("delete SA = %d", rec.Code)
	}

	// verify deleted
	rec = req(h, "GET", base+"/"+sa.Email, "")
	if rec.Code != 404 {
		t.Fatalf("get after delete = %d, want 404", rec.Code)
	}
}

func TestServiceAccountAccountIdRequired(t *testing.T) {
	h := New()
	rec := req(h, "POST", "/v1/projects/proj/serviceAccounts", `{}`)
	if rec.Code != 400 {
		t.Fatalf("account ID required = %d, want 400", rec.Code)
	}
}

func TestTestIamPermissions(t *testing.T) {
	h := New()
	rec := req(h, "POST", "/v1/projects/proj/serviceAccounts/sa@x:testIamPermissions", `{"permissions": ["iam.serviceaccounts.get"]}`)
	if !strings.Contains(rec.Body.String(), "iam.serviceaccounts.get") {
		t.Error("testIamPermissions should echo granted permissions")
	}
}
