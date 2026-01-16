package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/stefanpenner/gha-analyzer/pkg/core"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// This will eventually be populated with real OTel and other exporters
	pipeline := core.NewPipeline()
	_ = pipeline

	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement GitHub Webhook parsing and mapping to core.Trace
		fmt.Println("Received webhook (not yet implemented)")
		w.WriteHeader(http.StatusAccepted)
	})

	fmt.Printf("📡 Webhook server listening on :%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Server failed: %v\n", err)
		os.Exit(1)
	}
}

// TODO: Add webhook mapping logic to pkg/ingest/webhook
