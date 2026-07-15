package iam

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
	"github.com/corestack-io/corestack-gcp/internal/core/storage"
)

type ServiceAccount struct {
	Email       string `json:"email"`
	Name        string `json:"name"`
	ProjectId   string `json:"projectId"`
	UniqueId    string `json:"uniqueId"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	OAuth2      string `json:"oauth2ClientId,omitempty"`
	Etag        string `json:"etag"`
	Disabled    bool   `json:"disabled,omitempty"`
	createTime  string
}

type Key struct {
	Name            string `json:"name"`
	KeyId           string `json:"-"`
	PrivateKeyType  string `json:"privateKeyType,omitempty"`
	KeyAlgorithm    string `json:"keyAlgorithm"`
	PrivateKeyData  string `json:"privateKeyData,omitempty"`
	PublicKeyData   string `json:"publicKeyData,omitempty"`
	ValidAfterTime  string `json:"validAfterTime"`
	ValidBeforeTime string `json:"validBeforeTime"`
	KeyOrigin       string `json:"keyOrigin"`
	KeyType         string `json:"keyType"`
}

type Policy struct {
	Version  int             `json:"version"`
	Etag     string          `json:"etag"`
	Bindings []PolicyBinding `json:"bindings,omitempty"`
}

type PolicyBinding struct {
	Role    string   `json:"role"`
	Members []string `json:"members"`
}

type saKey struct{ Project, Email string }
type keykey struct{ Project, Email, KeyId string }

type Handler struct {
	accounts storage.Backend[saKey, *ServiceAccount]
	keys     storage.Backend[keykey, *Key]
	policies storage.Backend[string, *Policy]
}

func New() *Handler {
	return &Handler{
		accounts: storage.NewInMenory[saKey, *ServiceAccount](),
		keys:     storage.NewInMenory[keykey, *Key](),
		policies: storage.NewInMenory[string, *Policy](),
	}
}

func (h *Handler) Name() string {
	return "iam"
}

func (h *Handler) ClaimsPath(path string) bool {
	return strings.Contains(path, "/serviceAccounts")
}

type parsed struct {
	project      string
	identifier   string
	customMethod string
	keySubID     string
	isKeys       bool
	isKeysList   bool
}

func (h *Handler) parse(path string) (parsed, bool) {
	path = stripQuery(path)
	segs := strings.Split(strings.Trim(path, "/"), "/")
	var p parsed
	for i, s := range segs {
		if s == "projects" && i+1 < len(segs) {
			p.project = segs[i+1]
		}
		if s == "serviceAccounts" {
			rest := segs[i+1:]
			if len(rest) == 0 {
				return p, true
			}
			ident := rest[0]
			if ci := strings.LastIndex(ident, ":"); ci >= 0 {
				p.customMethod = ident[ci+1:]
				ident = ident[:ci]
			}
			p.identifier = ident
			if len(rest) > 2 && rest[1] == "keys" {
				p.isKeys = true
				if len(rest) >= 3 {
					p.keySubID = rest[2]
				} else {
					p.isKeysList = true
				}
			}
			return p, true
		}
	}
	return p, p.project != ""
}

func (h *Handler) Handle(w http.ResponseWriter, rc *protocol.RequestContext) {
	p, ok := h.parse(rc.Path)
	if !ok || p.project == "" {
		protocol.WriteError(w, protocol.InvalidArgument("malformed IAM path"))
		return
	}
	method := strings.ToUpper(rc.Method)
	body := parseBody(rc)

	switch p.customMethod {
	case "setIamPolicy":
		h.setIamPolicy(w, iamResource(p), body)
		return
	case "getIamPolicy":
		h.getIamPolicy(w, iamResource(p))
		return
	case "testIamPermissions":
		h.testIamPermissions(w, body)
		return
	}

	if p.isKeys {
		switch {
		case method == "POST":
			h.createKey(w, p)
		case method == "GET" && p.isKeysList:
			h.listKeys(w, p)
		case method == "DELETE":
			h.deleteKey(w, p)
		case method == "GET":
			h.getKey(w, p)
		default:
			protocol.WriteError(w, protocol.InvalidArgument("unsupported key op"))

		}
		return
	}

	switch {
	case p.identifier == "" && method == "POST":
		h.createServiceAccount(w, p, body)
	case p.identifier == "" && method == "GET":
		h.listServiceAccounts(w, p)
	case p.identifier != "" && method == "GET":
		h.getServiceAccount(w, p)
	case p.identifier != "" && (method == "PATCH" || method == "PUT"):
		h.patchServiceAccount(w, p, body)
	case p.identifier != "" && method == "DELETE":
		h.deleteServiceAccount(w, p)
	default:
		protocol.WriteError(w, protocol.InvalidArgument("unsupported IAM op"))
	}
}

// --- service account related functions---

func (h *Handler) createServiceAccount(w http.ResponseWriter, p parsed, body map[string]any) {
	accountID, _ := body["accountId"].(string)
	if accountID == "" {
		protocol.WriteError(w, protocol.InvalidArgument("accountId is required"))
		return
	}
	email := accountID + "@" + p.project + ".iam.gserviceaccount.com"
	if _, ok := h.accounts.Get(saKey{p.project, email}); ok {
		protocol.WriteError(w, protocol.AlreadyExists("service account exists: "+email))
		return
	}
	displayName, description := "", ""
	if sa, ok := body["serviceAccount"].(map[string]any); ok {
		displayName, _ = sa["displayName"].(string)
		description, _ = sa["description"].(string)
	}

	sa := &ServiceAccount{
		Email:       email,
		Name:        "projects/" + p.project + "/serviceAccounts/" + email,
		DisplayName: displayName,
		Description: description,
		ProjectId:   p.project,
		UniqueId:    genID(21),
		OAuth2:      genID(21),
		Etag:        etag(email),
		createTime:  nowRFC3339(),
	}

	h.accounts.Put(saKey{p.project, email}, sa)
	writeJSON(w, 200, sa)
}

func (h *Handler) resolve(p parsed) (*ServiceAccount, bool) {
	if sa, ok := h.accounts.Get(saKey{p.project, p.identifier}); ok {
		return sa, true
	}

	for _, sa := range h.accounts.Scan(func(k saKey) bool { return k.Project == p.project }) {
		if sa.UniqueId == p.identifier || sa.Email == p.identifier {
			return sa, true
		}
	}
	return nil, false
}

func (h *Handler) getServiceAccount(w http.ResponseWriter, p parsed) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	writeJSON(w, 200, sa)
}

func (h *Handler) listServiceAccounts(w http.ResponseWriter, p parsed) {
	items := h.accounts.Scan(func(sk saKey) bool { return sk.Project == p.project })
	writeJSON(w, 200, map[string]any{"accounts": items})
}

func (h *Handler) patchServiceAccount(w http.ResponseWriter, p parsed, body map[string]any) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	patch := body
	if m, ok := body["serviceAccount"].(map[string]any); ok {
		patch = m
	}
	if dn, ok := patch["displayName"].(string); ok {
		sa.DisplayName = dn
	}
	if d, ok := patch["description"].(string); ok {
		sa.Description = d
	}
	h.accounts.Put(saKey{p.project, sa.Email}, sa)
	writeJSON(w, 200, sa)
}

func (h *Handler) deleteServiceAccount(w http.ResponseWriter, p parsed) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	h.accounts.Delete(saKey{p.project, sa.Email})
	writeJSON(w, 200, map[string]any{})
}

// --- keys ---
func (h *Handler) createKey(w http.ResponseWriter, p parsed) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	keyID := genID(40)
	now := time.Now().UTC()
	k := &Key{
		Name:            sa.Name + "/keys/" + keyID,
		KeyId:           keyID,
		KeyAlgorithm:    "KEY_ALG_RSA_2048",
		ValidAfterTime:  now.Format(time.RFC3339),
		ValidBeforeTime: now.AddDate(10, 0, 0).Format(time.RFC3339),
		KeyOrigin:       "GOOGLE_PROVIDED",
		KeyType:         "USER_MANAGED",
		PrivateKeyType:  "TYPE_GOOGLE_CREDENTIALS_FILE",
		PrivateKeyData:  base64.StdEncoding.EncodeToString([]byte("fake-private-key-" + keyID)),
	}
	h.keys.Put(keykey{p.project, sa.Email, keyID}, k)
	writeJSON(w, 200, k)
}

func (h *Handler) listKeys(w http.ResponseWriter, p parsed) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	items := h.keys.Scan(func(k keykey) bool {
		return k.Project == p.project && k.Email == sa.Email
	})
	out := make([]*Key, 0, len(items))
	for _, k := range items {
		cp := *k
		cp.PrivateKeyData = ""
		out = append(out, &cp)
	}
	writeJSON(w, 200, map[string]any{"keys": out})
}

func (h *Handler) getKey(w http.ResponseWriter, p parsed) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	k, ok := h.keys.Get(keykey{p.project, sa.Email, p.keySubID})
	if !ok {
		protocol.WriteError(w, protocol.NotFound("key not found: "+p.keySubID))
		return
	}
	cp := *k
	cp.PrivateKeyData = ""
	writeJSON(w, 200, &cp)
}

func (h *Handler) deleteKey(w http.ResponseWriter, p parsed) {
	sa, ok := h.resolve(p)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("service account not found: "+p.identifier))
		return
	}
	if _, ok := h.keys.Get(keykey{p.project, sa.Email, p.keySubID}); !ok {
		protocol.WriteError(w, protocol.NotFound("key not found: "+p.keySubID))
		return
	}
	h.keys.Delete(keykey{p.project, sa.Email, p.keySubID})
	writeJSON(w, 200, map[string]any{})
}

// --- IAM Policy ---

func (h *Handler) getIamPolicy(w http.ResponseWriter, resource string) {
	pol, ok := h.policies.Get(resource)
	if !ok {
		pol = &Policy{Version: 1, Etag: base64.StdEncoding.EncodeToString([]byte("ACAB"))}
	}
	writeJSON(w, 200, pol)
}

func (h *Handler) setIamPolicy(w http.ResponseWriter, resource string, body map[string]any) {
	pol := &Policy{Version: 1, Etag: etag(resource + nowRFC3339())}
	if raw, ok := body["policy"]; ok {
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, pol)
		if pol.Etag == "" {
			pol.Etag = etag(resource + nowRFC3339())
		}
	}
	h.policies.Put(resource, pol)
	writeJSON(w, 200, pol)
}

func (h *Handler) testIamPermissions(w http.ResponseWriter, body map[string]any) {
	perms := []any{}
	if raw, ok := body["permissions"].([]any); ok {
		perms = raw
	}
	writeJSON(w, 200, map[string]any{"permissions": perms})
}

// --- Helpers ---

func iamResource(p parsed) string {
	return "projects/" + p.project + "/locations/" + p.identifier
}

func parseBody(rc *protocol.RequestContext) map[string]any {
	if rc.Body == nil {
		return map[string]any{}
	}
	data, _ := io.ReadAll(rc.Body)
	if len(data) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return map[string]any{}
	}
	return m
}

func stripQuery(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

func genID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const digits = "0123456789"
	for i := range b {
		b[i] = digits[b[i]%10]
	}
	return string(b)
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func etag(seed string) string {
	end := 4
	if len(seed) < end {
		end = len(seed)
	}
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(len(seed)) + seed[:end]))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
