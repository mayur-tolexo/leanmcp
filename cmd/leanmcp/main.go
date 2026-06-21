package main

import (
	"log"
	"net/http"
)

// healthHandler returns a handler that reports liveness with a 200 "ok".
func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// main wires the HTTP mux and starts the server. Real wiring is added in later phases.
func main() {
	mux := http.NewServeMux()
	mux.Handle("/healthz", healthHandler())
	mux.Handle("/readyz", healthHandler())
	log.Println("leanmcp listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
