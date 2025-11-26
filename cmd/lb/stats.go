package main

import (
	"encoding/json"
	"net/http"
)

// statsHandler returns the current status of the server pool
func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := serverPool.GetStats()
	writeJSON(w, stats)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	js, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}
