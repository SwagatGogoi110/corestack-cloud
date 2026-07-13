package operations

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
	"github.com/corestack-io/corestack-gcp/internal/core/storage"
)

type Operation struct {
	Name     string         `json:"name"`
	Done     bool           `json:"done"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Response map[string]any `json:"response,omitempty"`
	Error    *statusError   `json:"error,omitempty"`
}

type statusError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Handler struct {
	store storage.Backend[string, *Operation]
	seq   atomic.Int64
}

func New() *Handler {
	return &Handler{store: storage.NewInMenory[string, *Operation]()}
}

func (h *Handler) Name() string { return "operations" }

func (h *Handler) ClaimsPath(path string) bool {
	return strings.HasPrefix(path, "/v2/") && strings.Contains(path, "/operations")
}

func (h *Handler) Handle(w http.ResponseWriter, rc *protocol.RequestContext) {
	path := stripQuery(rc.Path)
	method := strings.ToUpper(rc.Method)

	if method == "POST" && strings.HasSuffix(path, ":wait") {
		name := strings.TrimSuffix(path, ":wait")
		h.getOne(w, name)
		return
	}

	idx := strings.Index(path, "/operations")
	if idx < 0 {
		protocol.WriteError(w, protocol.NotFound("not an operations path: "+path))
		return
	}
	tail := strings.TrimPrefix(path[idx:], "/operations")
	switch {
	case method == "GET" && (tail == "" || tail == "/"):
		parent := path[:idx]
		h.list(w, parent)
	case method == "GET":
		h.getOne(w, path)
	default:
		protocol.WriteError(w, protocol.InvalidArgument("unsupported operations method: "+method))
	}
}

func (h *Handler) getOne(w http.ResponseWriter, name string) {
	op, ok := h.store.Get(name)
	if !ok {
		protocol.WriteError(w, protocol.NotFound("operation not found: "+name))
		return
	}
	writeJSON(w, 200, op)
}

func (h *Handler) list(w http.ResponseWriter, parent string) {
	all := h.store.Scan(func(k string) bool { return strings.HasPrefix(k, parent+"/operations") })
	writeJSON(w, 200, map[string]any{"operations": all})
}

func (h *Handler) Register(parent string, done bool, response, metadata map[string]any) *Operation {
	id := "operation-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatInt(h.seq.Add(1), 10)
	name := parent + "/operations/" + id
	op := &Operation{Name: name, Done: done, Metadata: metadata, Response: response}
	h.store.Put(name, op)
	return op
}

func (h *Handler) Complete(name string, response map[string]any) (*Operation, bool) {
	op, ok := h.store.Get(name)
	if !ok {
		return nil, false
	}
	op.Done = true
	op.Response = response
	h.store.Put(name, op)
	return op, true
}

func (h *Handler) Fail(name string, code int, message string) (*Operation, bool) {
	op, ok := h.store.Get(name)
	if !ok {
		return nil, false
	}
	op.Done = true
	op.Error = &statusError{Code: code, Message: message}
	h.store.Put(name, op)
	return op, true
}

func stripQuery(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
