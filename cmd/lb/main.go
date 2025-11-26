package main

import (
	"flag"
	"fmt"
	"log"
	"net"
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

	// 1. Get a live backend via your method
	peer := serverPool.GetNextPeer()

	if peer != nil {
		// 2. If we found a server, forward the request
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
		}
	}
}

// isBackendAlive checks whether a backend is alive by establishing a TCP connection
func isBackendAlive(u *url.URL) bool {
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		log.Println("Site unreachable, error: ", err)
		return false
	}
	_ = conn.Close() // Close immediately, we just wanted to test
	return true
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

	// Parse servers
	tokens := strings.Split(serverList, ",")
	for _, tok := range tokens {
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
		Handler: http.HandlerFunc(lbHandler),
	}

	// Start health checking in a separate goroutine
	go healthCheck()

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
