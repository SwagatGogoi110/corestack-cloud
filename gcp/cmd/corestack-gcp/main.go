package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/corestack-io/corestack-gcp/internal/core/protocol"
)

var version = "dev"

func main()  {
	reg := protocol.NewRegistry()
	registerAll(reg)

	addr := ":4588"
	if v := os.Getenv("CORESTACK_GCP_LISTEN"); v != "" {
		addr = v
	}

	log.Printf("corestack-gcp %s listening on %s; services=%v", version, addr, reg.EnabledServices())
	if err := http.ListenAndServe(addr, topHandler(reg)); err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
}

func topHandler(reg *protocol.Registry) http.Handler {
	health := func(w http.ResponseWriter, _ *http.Request)  {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"emulator": "corestack-gcp",
			"version": version,
			"services": reg.EnabledServices(),
		})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/_floci/health", health)
	mux.Handle("/", reg)
	return mux
}

func registerAll(reg *protocol.Registry) {
//	reg.Register(protocol.ServiceDescriptor{Name: "gcs", Protocol: protocol.REST}, gcs.New())
}