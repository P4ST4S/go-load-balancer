package main

import (
	"encoding/json"
	"net/http"
)

// statsHandler returns the current status of the server pool
func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := serverPool.GetStats()

	data, err := json.Marshal(stats)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
