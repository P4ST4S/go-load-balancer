package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/P4ST4S/go-load-balancer/core"
)

// Configuration constants
const (
	RetryAttempts int = 3
)

// lbHandler is the orchestrator for each incoming request
func lbHandler(w http.ResponseWriter, r *http.Request) {
	// 0. Ignore favicon requests (browser noise)
	if r.URL.Path == "/favicon.ico" {
		http.Error(w, "No favicon", http.StatusNotFound)
		return
	}

	// 1. Get the backend with the least active connections
	peer := serverPool.GetLeastConnPeer()

	if peer != nil {
		// 2. Increment connection counter, ensure decrement after response
		peer.IncConn()
		defer peer.DecConn()

		// Forward the request
		peer.ReverseProxy.ServeHTTP(w, r)
		return
	}

	// 3. If no server is available (GetNextPeer returned nil)
	http.Error(w, "Service not available", http.StatusServiceUnavailable)
}

var serverPool core.ServerPool

// healthCheck pings the backends and updates their status
func healthCheck(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	// Worker pool for stats updates
	jobs := make(chan *core.Backend, len(serverPool.Backends))
	for i := 0; i < 3; i++ { // 3 workers
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case b := <-jobs:
					updateBackendStatsFunc(b)
				}
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, b := range serverPool.Backends {
				alive := isBackendAlive(b.URL)

				if b.IsAlive() != alive {
					status := "up"
					if !alive {
						status = "down"
					}
					log.Printf("Status change: %s [%s]", b.URL, status)

					b.SetAlive(alive)
				}

				if alive {
					// Non-blocking send to avoid blocking the health check loop
					select {
					case jobs <- b:
					default:
						log.Printf("Worker pool full, skipping stats update for %s", b.URL)
					}
				}
			}
		}
	}
}

type HealthResponse struct {
	MemoryUsage uint64 `json:"memory_usage"`
}

var updateBackendStatsFunc = updateBackendStats

func updateBackendStats(b *core.Backend) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(b.URL.String() + "/health")
	if err != nil {
		log.Printf("Error fetching stats from %s: %s", b.URL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error fetching stats from %s: status %d", b.URL, resp.StatusCode)
		return
	}

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		log.Printf("Error decoding stats from %s: %s", b.URL, err)
		return
	}

	b.SetMemoryUsage(health.MemoryUsage)
}

// isBackendAlive checks whether a backend is alive by establishing a TCP connection
func isBackendAlive(u *url.URL) bool {
	timeout := 2 * time.Second
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(u.String())
	if err != nil {
		log.Println("Site unreachable, error: ", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true
	}
	return false
}

func main() {
	var serverList string
	var port int

	flag.StringVar(&serverList, "backends", "", "Load balanced backends, use commas to separate")
	flag.IntVar(&port, "port", 3030, "Port to serve")
	flag.Parse()

	if len(serverList) == 0 {
		log.Fatal("Please provide one or more backends using -backends")
	}

	server, err := setupServer(serverList, port)
	if err != nil {
		log.Fatal(err)
	}

	// Start health checking in a separate goroutine
	go healthCheck(context.Background(), 20*time.Second)

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func setupServer(serverList string, port int) (*http.Server, error) {
	// Parse servers
	tokens := strings.SplitSeq(serverList, ",")
	for tok := range tokens {
		serverUrl, err := url.Parse(tok)
		if err != nil {
			return nil, err
		}

		// Create the Proxy
		proxy := httputil.NewSingleHostReverseProxy(serverUrl)

		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
			log.Printf("[%s] %s\n", serverUrl.Host, e.Error())
		}

		// Add to pool
		serverPool.AddBackend(&core.Backend{
			URL:          serverUrl,
			Alive:        true,
			ReverseProxy: proxy,
			StartTime:    time.Now(),
		})
		log.Printf("Configured server: %s\n", serverUrl)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", lbHandler)
	mux.HandleFunc("/stats", statsHandler)

	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
		// Timeouts to prevent Slow Loris attacks and resource leaks
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return server, nil
}
