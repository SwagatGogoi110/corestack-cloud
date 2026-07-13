package gcs

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
	"github.com/corestack-io/corestack-gcp/internal/core/storage"
)

const defaultProjectID = "corestack=local"

type bucketKey struct{ Name string }
type objectKey struct {
	Bucket string
	Name   string
}

type Bucket struct {
	Kind           string            `json:"kind"`
	ID             string            `json:"id"`
	SelfLink       string            `json:"selfLink"`
	Name           string            `json:"name"`
	ProjectNumber  string            `json:"projectNumber,omitempty"`
	Metageneration string            `json:"metageneration"`
	Location       string            `json:"location"`
	StorageClass   string            `json:"storageClass"`
	TimeCreated    string            `json:"timeCreated"`
	Updated        string            `json:"updated"`
	Etag           string            `json:"etag"`
	ProjectID      string            `json:"-"`
	Labels         map[string]string `json:"labels,omitempty"`
}

type Object struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	SelfLink       string `json:"selfLink"`
	Name           string `json:"name"`
	Bucket         string `json:"bucket"`
	Generation     string `json:"generation"`
	Metageneration string `json:"metageneration"`
	ContentType    string `json:"contentType,omitempty"`
	StorageClass   string `json:"storageClass"`
	Size           string `json:"size"`
	TimeCreated    string `json:"timeCreated"`
	Updated        string `json:"updated"`
	Crc32c         string `json:"crc32c,omitempty"`
	Md5Hash        string `json:"md5Hash,omitempty"`
	MediaLink      string `json:"mediaLink,omitempty"`
	Etag           string `json:"etag"`
	data           []byte
}

type Handler struct {
	buckets storage.Backend[bucketKey, *Bucket]
	objects storage.Backend[objectKey, *Object]
}

func New() *Handler {
	return &Handler{
		buckets: storage.NewInMenory[bucketKey, *Bucket](),
		objects: storage.NewInMenory[objectKey, *Object](),
	}
}

func (h *Handler) Name() string { return "gcs" }

func (h *Handler) ClaimsPath(path string) bool {
	return strings.HasPrefix(path, "/storage/v1/b")
}

func (h *Handler) Handle(w http.ResponseWriter, rc *protocol.RequestContext) {
	path := rc.Path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}

	rest := strings.TrimPrefix(path, "/storage/v1/b")
	rest = strings.TrimPrefix(rest, "/")
	method := strings.ToUpper(rc.Method)

	if bucket, objTail, isObj := splitObjectPath(rest); isObj {
		h.handleObject(w, rc, method, bucket, objTail)
		return
	}

	switch {
	case rest == "" && method == "POST":
		h.createBucket(w, rc)
	case rest == "" && method == "GET":
		h.listBuckets(w, rc)
	case rest != "" && method == "GET":
		h.getBucket(w, rest)
	case rest != "" && (method == "PATCH" || (method == "POST" && strings.EqualFold(rc.Header.Get("X-HTTP-Method-Override"), "PATCH"))):
		h.patchBucket(w, rc, rest)
	case rest != "" && method == "DELETE":
		h.deleteBucket(w, rest)
	default:
		protocol.WriteError(w, protocol.InvalidArgument("unsupported GCS bucket operation"))
	}
}

func splitObjectPath(rest string) (string, string, bool) {
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) >= 2 && parts[1] == "o" {
		obj := ""
		if len(parts) == 3 {
			obj = parts[2]
		}
		return parts[0], obj, true
	}
	return "", "", false
}

// --- buckets ---

func (h *Handler) createBucket(w http.ResponseWriter, rc *protocol.RequestContext) {
	body := parseBody(rc)
	name, _ := body["name"].(string)
	if strings.TrimSpace(name) == "" {
		protocol.WriteError(w, protocol.InvalidArgument("bucket name is required"))
		return
	}
	if _, ok := h.buckets.Get(bucketKey{name}); ok {
		protocol.WriteError(w, protocol.AlreadyExists("bucket alreadt exists: "+name))
		return
	}
	project := rc.Query["project"]
	if project == "" {
		project = defaultProjectID
	}
	location := "US"
	if l, ok := body["location"].(string); ok && l != "" {
		location = strings.ToUpper(l)
	}
	storageClass := "STANDARD"
	if s, ok := body["storageClass"].(string); ok && s != "" {
		storageClass = s
	}
	now := nowRFC3339()
	b := &Bucket{
		Kind: "storage#bucket", ID: name, Name: name,
		SelfLink:  baseURL(rc) + "/storage/v1/b/" + name,
		ProjectID: project, Metageneration: "1",
		Location: location, StorageClass: storageClass,
		TimeCreated: now, Updated: now,
		Etag:   etag(name + now),
		Labels: parseStringMap(body["labels"]),
	}
	h.buckets.Put(bucketKey{name}, b)
	writeJSON(w, 200, b)
}

func (h *Handler) deleteBucket(w http.ResponseWriter, rest string) {
	if _, ok := h.buckets.Get(bucketKey{rest}); !ok {

		protocol.WriteError(w, protocol.NotFound("bucket not found: "+rest))
		return
	}
	h.buckets.Delete(bucketKey{rest})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) patchBucket(w http.ResponseWriter, rc *protocol.RequestContext, rest string) {
	b, ok := h.buckets.Get(bucketKey{rest})
	if !ok {
		protocol.WriteError(w, protocol.NotFound("bucket not found: "+rest))
		return
	}
	patch := parseBody(rc)
	if sc, ok := patch["storageClass"].(string); ok && sc != "" {
		b.StorageClass = sc
	}
	if labels := parseStringMap(patch["labels"]); labels != nil {
		b.Labels = labels
	}
	b.Metageneration = bumpGeneration(b.Metageneration)
	b.Updated = nowRFC3339()
	h.buckets.Put(bucketKey{rest}, b)
	writeJSON(w, 200, b)
}

func (h *Handler) getBucket(w http.ResponseWriter, rest string) {
	b, ok := h.buckets.Get(bucketKey{rest})
	if !ok {
		protocol.WriteError(w, protocol.NotFound("bucket not found: "+rest))
		return
	}
	writeJSON(w, 200, b)
}

func (h *Handler) listBuckets(w http.ResponseWriter, rc *protocol.RequestContext) {
	project := rc.Query["project"]
	all := h.buckets.Scan(func(bk bucketKey) bool { return true })
	filtered := make([]*Bucket, 0, len(all))
	for _, b := range all {
		if project == "" || b.ProjectID == project {
			filtered = append(filtered, b)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	resp := map[string]any{"kind": "storage#buckets"}
	if len(filtered) > 0 {
		resp["items"] = filtered
	}
	writeJSON(w, 200, resp)
}

// --- objects ---

func (h *Handler) handleObject(w http.ResponseWriter, rc *protocol.RequestContext, method, bucket, objName string) {
	if _, ok := h.buckets.Get(bucketKey{bucket}); !ok {
		protocol.WriteError(w, protocol.NotFound("bucket not found: "+bucket))
		return
	}
	switch {
	case objName == "" && method == "GET":
		h.listObjects(w, bucket)
	case objName != "" && method == "GET":
		if rc.Query["alt"] == "media" {
			h.downloadObjects(w, bucket, objName)
		} else {
			h.getObjects(w, bucket, objName)
		}
	case objName != "" && method == "DELETE":
		h.deleteObject(w, bucket, objName)
	default:
		protocol.WriteError(w, protocol.InvalidArgument("unsupported GCS object operation"))
	}
}

func (h *Handler) deleteObject(w http.ResponseWriter, bucket string, name string) {
	if _, ok := h.objects.Get(objectKey{bucket, name}); !ok {
		protocol.WriteError(w, protocol.NotFound("object not found: "+name))
		return
	}
	h.objects.Delete(objectKey{bucket, name})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getObjects(w http.ResponseWriter, bucket string, name string) {
	o, ok := h.objects.Get(objectKey{bucket, name})
	if !ok {
		protocol.WriteError(w, protocol.NotFound("object not found: "+name))
		return
	}
	writeJSON(w, 200, o)
}

func (h *Handler) downloadObjects(w http.ResponseWriter, bucket string, name string) {
	o, ok := h.objects.Get(objectKey{bucket, name})
	if !ok {
		protocol.WriteError(w, protocol.NotFound("object not found: "+name))
		return
	}
	ct := o.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(200)
	_, _ = w.Write(o.data)
}

func (h *Handler) listObjects(w http.ResponseWriter, bucket string) {
	items := h.objects.Scan(func(k objectKey) bool { return k.Bucket == bucket })
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	resp := map[string]any{"kind": "storage#objects"}
	if len(items) > 0 {
		resp["items"] = items
	}
	writeJSON(w, 200, resp)
}

func (h *Handler) InsertObject(rc *protocol.RequestContext, bucket, name, contentType string, data []byte) (*Object, *protocol.Error) {
	if _, ok := h.buckets.Get(bucketKey{bucket}); !ok {
		return nil, protocol.NotFound("bucket not found: " + bucket)
	}
	now := nowRFC3339()
	gen := strconv.FormatInt(time.Now().UnixNano(), 10)
	sum := md5.Sum(data)
	crc := crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli))
	crcBytes := []byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)}
	o := &Object{
		Kind: "storage/object", ID: bucket + "/" + name + "/" + gen,
		Name: name, Bucket: bucket, Generation: gen, Metageneration: "1",
		ContentType: contentType, StorageClass: "STANDARD",
		Size: strconv.Itoa(len(data)), TimeCreated: now, Updated: now,
		Md5Hash:   base64.StdEncoding.EncodeToString(sum[:]),
		Crc32c:    base64.StdEncoding.EncodeToString(crcBytes),
		SelfLink:  baseURL(rc) + "/storage/v1/b/" + bucket + "/o" + name,
		MediaLink: baseURL(rc) + "/download/storage/v1/b/" + bucket + "/o/" + name + "?alt=media",
		Etag:      etag(bucket + name + gen),
		data:      data,
	}
	h.objects.Put(objectKey{bucket, name}, o)
	return o, nil
}

// --- helpers ---

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

func parseStringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}

func baseURL(rc *protocol.RequestContext) string {
	scheme := "http"
	if rc.Raw != nil && rc.Raw.TLS != nil {
		scheme = "https"
	}
	host := rc.Header.Get("Host")
	if host == "" && rc.Raw != nil {
		host = rc.Raw.Host
	}
	if host == "" {
		host = "localhost:4443"
	}
	return scheme + "://" + host
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func bumpGeneration(s string) string {
	n, _ := strconv.Atoi(s)
	return strconv.Itoa(n + 1)
}

func etag(seed string) string {
	sum := md5.Sum([]byte(seed))
	return fmt.Sprintf("%x", sum[:6])
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
