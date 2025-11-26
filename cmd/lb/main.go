package main

import (
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
// waiting 20s before checking again
func healthCheck() {
	t := time.NewTicker(20 * time.Second)

	// Worker pool for stats updates
	jobs := make(chan *core.Backend, len(serverPool.Backends))
	for i := 0; i < 3; i++ { // 3 workers
		go func() {
			for b := range jobs {
				updateBackendStats(b)
			}
		}()
	}

	for range t.C {
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

type HealthResponse struct {
	MemoryUsage uint64 `json:"memory_usage"`
}

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

	// Pass the server list as an argument for easy testing
	// e.g.: -backends=http://localhost:8081,http://localhost:8082
	flag.StringVar(&serverList, "backends", "", "Load balanced backends, use commas to separate")
	flag.IntVar(&port, "port", 3030, "Port to serve")
	flag.Parse()

	if len(serverList) == 0 {
		log.Fatal("Please provide one or more backends using -backends")
	}

	// Register handlers
	http.HandleFunc("/", lbHandler)
	http.HandleFunc("/stats", statsHandler)
	// The /stats endpoint is intentionally public in production environments.
	// If you need to restrict access, add authentication here.

	// Parse servers
	tokens := strings.SplitSeq(serverList, ",")
	for tok := range tokens {
		serverUrl, err := url.Parse(tok)
		if err != nil {
			log.Fatal(err)
		}

		// Create the Proxy
		proxy := httputil.NewSingleHostReverseProxy(serverUrl)

		// Customize the proxy (Optional but recommended for Senior level)
		proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, e error) {
			log.Printf("[%s] %s\n", serverUrl.Host, e.Error())
			// Here we could add retry logic
			// Or mark the backend as dead immediately
		}

		// Add to pool
		serverPool.AddBackend(&core.Backend{
			URL:          serverUrl,
			Alive:        true, // We assume they are alive at startup
			ReverseProxy: proxy,
			StartTime:    time.Now(),
		})
		log.Printf("Configured server: %s\n", serverUrl)
	}

	// Create HTTP server
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: nil, // Use DefaultServeMux
		// Timeouts to prevent Slow Loris attacks and resource leaks
		ReadHeaderTimeout: 2 * time.Second,  // Fast fail for slow headers (Slow Loris)
		ReadTimeout:       15 * time.Second, // Time to read the full request
		WriteTimeout:      15 * time.Second, // Time to write the full response
		IdleTimeout:       60 * time.Second, // Keep-alive connections
	}

	// Start health checking in a separate goroutine
	go healthCheck()

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
