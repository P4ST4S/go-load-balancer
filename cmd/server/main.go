package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
)

type HealthResponse struct {
	MemoryUsage uint64 `json:"memory_usage"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from backend! I am running on %s\n", os.Getenv("HOSTNAME"))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		resp := HealthResponse{
			MemoryUsage: m.Alloc,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	log.Printf("Backend server starting on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
