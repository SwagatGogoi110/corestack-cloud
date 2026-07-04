package protocol

import (
	"io"
	"net/http"
	"strings"
)

type RequestContext struct {
	ProjectID string
	Method    string
	Path      string
	Header    http.Header
	Query     map[string]string
	Body      io.ReadCloser
	Raw       *http.Request
}

type Protocol int

const (
	REST Protocol = iota
	GRPC
)

type Handler interface {
	Name() string
	Handle(w http.ResponseWriter, rc *RequestContext)
}

type ServiceDescriptor struct {
	Name       string
	Protocol   Protocol
	Enabled    bool
	HostToken  string
	PathPrefix string
	handler    Handler
}

type Registry struct {
	descriptors []ServiceDescriptor
	byName      map[string]*ServiceDescriptor
}

func NewRegistry() *Registry {
	return &Registry{byName: map[string]*ServiceDescriptor{}}
}

func (r *Registry) Register(d ServiceDescriptor, h Handler) {
	d.handler = h
	d.Enabled = true
	r.descriptors = append(r.descriptors, d)
	r.byName[d.Name] = &r.descriptors[len(r.descriptors)-1]
}

func (r *Registry) EnabledServices() []string {
	out := make([]string, 0, len(r.descriptors))
	for _, d := range r.descriptors {
		if d.Enabled {
			out = append(out, d.Name)
		}
	}
	return out
}

func (r *Registry) routableServices() []*ServiceDescriptor {
	var out []*ServiceDescriptor
	for i := range r.descriptors {
		d := &r.descriptors[i]
		if d.Enabled && d.PathPrefix != "" {
			out = append(out, d)
		}
	}
	return out
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	method := req.Method
	if mo := req.Header.Get("X-HTTP-Method-Override"); mo != "" {
		method = strings.ToUpper(mo)
	} else if mo := req.URL.Query().Get("$method"); mo != "" {
		method = strings.ToUpper(mo)
	}

	path := req.URL.Path
	path = r.applyHostRouting(req, path)

	rc := &RequestContext{
		ProjectID: extractProjectID(path),
		Method:    method,
		Path:      path,
		Header:    req.Header,
		Query:     firstQueryValues(req),
		Body:      req.Body,
		Raw:       req,
	}

	if h := r.resolve(path); h != nil {
		h.Handle(w, rc)
		return
	}
	WriteError(w, NotFound("No service handles path: "+path))
}

func (r *Registry) applyHostRouting(req *http.Request, path string) string {
	if !strings.HasPrefix(path, "/v1/") {
		return path
	}
	hostLabel := firstHostLabel(req)
	for _, d := range r.routableServices() {
		if strings.HasPrefix(path, d.PathPrefix+"/") {
			return path
		}
		if d.HostToken != "" && d.HostToken == hostLabel {
			return d.PathPrefix + path
		}
	}
	return path
}

func (r *Registry) resolve(path string) Handler {
	var best *ServiceDescriptor
	for i := range r.descriptors {
		d := &r.descriptors[i]
		if !d.Enabled || d.PathPrefix == "" {
			continue
		}
		if strings.HasPrefix(path, d.PathPrefix+"/") || path == d.PathPrefix {
			if best == nil || len(d.PathPrefix) > len(best.PathPrefix) {
				best = d
			}
		}
	}
	if best != nil {
		return best.handler
	}

	for i := range r.descriptors {
		d := &r.descriptors[i]
		if !d.Enabled || d.PathPrefix != "" {
			continue
		}
		if c, ok := d.handler.(PathClaimer); ok && c.ClaimsPath(path) {
			return d.handler
		}
	}
	return nil
}

type PathClaimer interface {
	ClaimsPath(path string) bool
}

func (r *Registry) Claimants(path string) []string {
	var names []string
	for i := range r.descriptors {
		d := &r.descriptors[i]
		if !d.Enabled || d.PathPrefix == "" {
			continue
		}
		if strings.HasPrefix(path, d.PathPrefix+"/") || path == d.PathPrefix {
			names = append(names, d.Name)
		}
	}
	for i := range r.descriptors {
		d := &r.descriptors[i]
		if !d.Enabled || d.PathPrefix == "" {
			continue
		}
		if c, ok := d.handler.(PathClaimer); ok && c.ClaimsPath(path) {
			names = append(names, d.Name)
		}
	}
	return names
}

func extractProjectID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func firstHostLabel(req *http.Request) string {
	host := req.Header.Get("Host")
	if host == "" {
		host = req.Host
	}
	if host == "" {
		return ""
	}
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if i := strings.IndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}

func firstQueryValues(req *http.Request) map[string]string {
	out := map[string]string{}
	for k, v := range req.URL.Query() {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}
