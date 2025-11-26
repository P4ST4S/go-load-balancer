package main

import (
	"encoding/json"
	"net/http"
)

// statsHandler returns the current status of the server pool
func statsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	stats := serverPool.GetStats()

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
