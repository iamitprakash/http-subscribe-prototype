package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// StockEvent represents a real-time event pushed over the stream.
type StockEvent struct {
	Ticker    string    `json:"ticker"`
	Price     float64   `json:"price"`
	Timestamp time.Time `json:"timestamp"`
}

// SubscriptionRequest represents the body of the SUBSCRIBE request.
type SubscriptionRequest struct {
	Ticker     string `json:"ticker"`
	IntervalMS int    `json:"interval_ms"`
}

func main() {
	mode := flag.String("mode", "demo", "Execution mode: 'demo', 'server', or 'client'")
	addr := flag.String("addr", "127.0.0.1:8080", "Server address to listen on or connect to")
	ticker := flag.String("ticker", "GOOG", "Ticker to subscribe to (client mode only)")
	flag.Parse()

	switch *mode {
	case "server":
		runServer(*addr)
	case "client":
		runClient(*addr, *ticker)
	case "demo":
		runDemo()
	default:
		fmt.Printf("Unknown mode: %s. Use 'demo', 'server', or 'client'\n", *mode)
		os.Exit(1)
	}
}

// runServer starts the SUBSCRIBE-enabled streaming server.
func runServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", subscribeHandler)

	fmt.Printf("[SERVER] Starting SUBSCRIBE server on http://%s...\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[SERVER] Failed to start server: %v", err)
	}
}

// subscribeHandler handles incoming stream subscriptions.
// It requires the HTTP SUBSCRIBE method (RFC 10008) and streams JSON stock events.
func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Enforce the new SUBSCRIBE HTTP method string
	if r.Method != "SUBSCRIBE" {
		w.Header().Set("Allow", "SUBSCRIBE")
		http.Error(w, "Method Not Allowed. This endpoint requires SUBSCRIBE.", http.StatusMethodNotAllowed)
		return
	}

	// 2. Validate content type of the subscription filter body
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// 3. Parse subscription parameters from the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var subReq SubscriptionRequest
	if err := json.Unmarshal(body, &subReq); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Fallback to defaults if values are missing or invalid
	if subReq.Ticker == "" {
		subReq.Ticker = "GOOG"
	}
	if subReq.IntervalMS <= 0 {
		subReq.IntervalMS = 1000
	}

	fmt.Printf("[SERVER] New subscription request accepted for ticker: %q (interval: %dms)\n", subReq.Ticker, subReq.IntervalMS)

	// 4. Set headers to establish a persistent stream connection
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Prevents buffering in Nginx reverse-proxies

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported by server", http.StatusInternalServerError)
		return
	}

	// 5. Send initial handshake connection OK
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": connection established\n\n")
	flusher.Flush()

	// 6. Set up event loops and tickers
	tickerInterval := time.Duration(subReq.IntervalMS) * time.Millisecond
	eventTicker := time.NewTicker(tickerInterval)
	defer eventTicker.Stop()

	// We send a keep-alive comment every 3 ticks
	heartbeatTicker := time.NewTicker(tickerInterval * 3)
	defer heartbeatTicker.Stop()

	// Mock price tracker
	currentPrice := 150.0 + (rand.Float64() * 100.0)

	// Listen to stream cancellations or client disconnects
	for {
		select {
		case <-r.Context().Done():
			// The connection was closed by the client or interrupted by an intermediary
			fmt.Printf("[SERVER] Subscription closed. Connection context cancelled. Releasing resources for %s...\n", subReq.Ticker)
			return

		case <-eventTicker.C:
			// Generate stock event with minor price variation
			priceDelta := (rand.Float64() - 0.5) * 2.0
			currentPrice += priceDelta
			if currentPrice < 1.0 {
				currentPrice = 1.0
			}

			event := StockEvent{
				Ticker:    subReq.Ticker,
				Price:     currentPrice,
				Timestamp: time.Now(),
			}

			jsonBytes, err := json.Marshal(event)
			if err != nil {
				log.Printf("[SERVER] Failed to marshal event: %v", err)
				return
			}

			// Format as Server-Sent Event: "data: <json>\n\n"
			fmt.Fprintf(w, "data: %s\n\n", string(jsonBytes))
			flusher.Flush()

		case <-heartbeatTicker.C:
			// Send a keep-alive message comment chunk (prefixed with ':')
			// This tells proxies/CDNs the stream is still active
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

// runClient opens a SUBSCRIBE connection and reads events.
func runClient(addr string, ticker string) {
	// Create client context that we can cancel to stop the stream
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Build subscription parameters body
	subReq := SubscriptionRequest{
		Ticker:     ticker,
		IntervalMS: 500, // Request fast updates
	}
	jsonBytes, err := json.Marshal(subReq)
	if err != nil {
		log.Fatalf("[CLIENT] Failed to marshal request: %v", err)
	}

	url := fmt.Sprintf("http://%s/subscribe", addr)
	fmt.Printf("[CLIENT] Preparing SUBSCRIBE request with body: %s\n", string(jsonBytes))

	// 2. Build HTTP request with SUBSCRIBE verb
	req, err := http.NewRequestWithContext(ctx, "SUBSCRIBE", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		log.Fatalf("[CLIENT] Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	fmt.Printf("[CLIENT] Connecting to subscription stream at %s...\n", url)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("[CLIENT] Connection failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("[CLIENT] Server returned error status: %s", resp.Status)
	}

	fmt.Println("[CLIENT] Subscription active! Parsing stream:")
	reader := bufio.NewReader(resp.Body)
	eventsCount := 0

	// 3. Scan the incoming stream chunks line by line
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("[CLIENT] Stream reached EOF.")
			} else {
				log.Printf("[CLIENT] Read error: %v", err)
			}
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle comments (heartbeats) starting with ':'
		if strings.HasPrefix(line, ":") {
			fmt.Printf("[CLIENT] Heartbeat comment received: %q\n", line)
			continue
		}

		// Handle data chunks starting with 'data: '
		if strings.HasPrefix(line, "data: ") {
			eventJSON := strings.TrimPrefix(line, "data: ")
			var event StockEvent
			if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
				fmt.Printf("[CLIENT] Raw data chunk (unmarshal failed): %s\n", eventJSON)
				continue
			}

			fmt.Printf("[CLIENT] Event #%d: [%s] Price = $%.2f at %s\n",
				eventsCount+1, event.Ticker, event.Price, event.Timestamp.Format("15:04:05.000"))

			eventsCount++
			// Auto-disconnect after receiving 5 events to show clean client termination
			if eventsCount >= 5 {
				fmt.Println("[CLIENT] Limit reached (5 events). Initiating client disconnect...")
				cancel() // Cancels the request context, which tears down the connection
				break
			}
		}
	}
}

// runDemo runs the combined server-client prototype demo.
func runDemo() {
	fmt.Println("[DEMO] Starting SUBSCRIBE protocol prototype demonstration...")

	// Listen on ephemeral port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("[DEMO] Failed to allocate port: %v", err)
	}
	addr := listener.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/subscribe", subscribeHandler)

	srv := &http.Server{
		Handler: mux,
	}

	// Start server in a background goroutine
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[SERVER] Server crashed: %v", err)
		}
	}()
	fmt.Printf("[SERVER] Started in background on http://%s\n", addr)

	// Wait briefly for server startup
	time.Sleep(100 * time.Millisecond)

	// Run client subscription against local server
	runClient(addr, "GOOG")

	// Wait briefly to observe server logs after client disconnect
	time.Sleep(200 * time.Millisecond)

	// Shut down server cleanly
	fmt.Println("[DEMO] Shutting down background server...")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[DEMO] Server shutdown error: %v", err)
	}
	fmt.Println("[DEMO] Demonstration completed successfully.")
}
