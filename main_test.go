package main

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubscribeHandler_MethodNotAllowed(t *testing.T) {
	req, err := http.NewRequest("GET", "/subscribe", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(subscribeHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusMethodNotAllowed {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusMethodNotAllowed)
	}

	allowHeader := rr.Header().Get("Allow")
	if allowHeader != "SUBSCRIBE" {
		t.Errorf("expected Allow header to be 'SUBSCRIBE', got %q", allowHeader)
	}
}

func TestSubscribeHandler_UnsupportedMediaType(t *testing.T) {
	req, err := http.NewRequest("SUBSCRIBE", "/subscribe", bytes.NewBuffer([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	// Missing Content-Type
	req.Header.Set("Content-Type", "text/plain")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(subscribeHandler)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusUnsupportedMediaType {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnsupportedMediaType)
	}
}

func TestSubscribeHandler_SuccessAndClose(t *testing.T) {
	// Create context that we can cancel to terminate the handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	payload := []byte(`{"ticker": "AAPL", "interval_ms": 10}`)
	req, err := http.NewRequestWithContext(ctx, "SUBSCRIBE", "/subscribe", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	// We use httptest.NewServer to test full streaming flusher behaviors
	server := httptest.NewServer(http.HandlerFunc(subscribeHandler))
	defer server.Close()

	client := &http.Client{}
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(server.URL, "http://")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 OK, got %v", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %q", contentType)
	}

	reader := bufio.NewReader(resp.Body)

	// 1. Read connection established comment
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read handshake: %v", err)
	}
	if !strings.Contains(line, "connection established") {
		t.Errorf("expected connection established comment, got %q", line)
	}

	// 2. Read first data event
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read first event line: %v", err)
	}
	// We read until blank line (double newline ends SSE event)
	for line == "\n" || line == "\r\n" {
		line, _ = reader.ReadString('\n')
	}

	if !strings.HasPrefix(line, "data: ") {
		t.Errorf("expected event chunk to start with 'data: ', got %q", line)
	}

	if !strings.Contains(line, "AAPL") {
		t.Errorf("expected event payload to contain AAPL ticker, got %q", line)
	}

	// 3. Cancel context to ensure server releases resources cleanly
	cancel()

	// Wait briefly to ensure context cancellation propagates
	time.Sleep(50 * time.Millisecond)
}
