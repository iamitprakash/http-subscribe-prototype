# HTTP SUBSCRIBE Method Go Prototype

This repository contains a working prototype of the HTTP `SUBSCRIBE` method as proposed in the submitted IETF Internet-Draft `draft-prakash-http-subscribe-00`.

The prototype showcases how an HTTP-native real-time event streaming connection can be established using a custom `SUBSCRIBE` request verb, supporting request body parameters for client-side filtering while preserving standard HTTP semantics and intermediate infrastructure support (caching bypass, reverse-proxy flushes).

---

## Key Features

1.  **Semantic `SUBSCRIBE` Verb:** The server listens for and enforces the custom HTTP method `SUBSCRIBE` for streaming endpoints.
2.  **Request Body Filtering:** The client sends subscription criteria (e.g., specific ticker filters and stream intervals) directly in the HTTP request body as JSON, avoiding query string limits and security logging leaks.
3.  **Real-Time Streaming:** The server stream matches the `text/event-stream` format (Server-Sent Events) with automatic flushes to prevent proxy-level buffering.
4.  **Heartbeats & Keep-Alive:** Periodic empty comment heartbeats (`: keep-alive`) are injected to ensure standard CDNs and reverse proxies don't prematurely close the idle connection.
5.  **Graceful Resource Management:** The server monitors client connection cancellation context (`r.Context().Done()`) to immediately stop streaming and release CPU/memory resources when a client cancels or disconnects.

---

## Code Overview

- [go.mod](go.mod): Go module definition.
- [main.go](main.go): The complete Go implementation of:
  - **Server:** Parses `"SUBSCRIBE"` methods, configures connection streams, and sends stock price changes.
  - **Client:** Connects via `"SUBSCRIBE"`, parses the event-stream, and shuts down after receiving 5 events.
  - **Demo Mode:** Runs both server and client concurrently on an ephemeral port to show a complete lifecycle demonstration.
- [main_test.go](main_test.go): Tests standard handler routing, HTTP status responses, custom method constraints, and connection teardown.

---

## How to Run

### Prerequisite
Make sure you have [Go](https://go.dev/doc/install) installed (Go 1.22+ is recommended).

### 1. Run the Combined Demo
The default execution mode fires up both the server and client concurrently, log transitions, streams stock price ticks, and terminates:
```bash
go run main.go
```

**Expected output:**
```text
[DEMO] Starting SUBSCRIBE protocol prototype demonstration...
[SERVER] Started in background on http://127.0.0.1:51234
[CLIENT] Preparing SUBSCRIBE request with body: {"ticker":"GOOG","interval_ms":500}
[CLIENT] Connecting to subscription stream at http://127.0.0.1:51234/subscribe...
[SERVER] New subscription request accepted for ticker: "GOOG" (interval: 500ms)
[CLIENT] Subscription active! Parsing stream:
[CLIENT] Heartbeat comment received: ": connection established"
[CLIENT] Event #1: [GOOG] Price = $183.45 at 15:30:00.500
[CLIENT] Event #2: [GOOG] Price = $184.22 at 15:30:01.000
[CLIENT] Heartbeat comment received: ": keep-alive"
[CLIENT] Event #3: [GOOG] Price = $183.98 at 15:30:01.500
[CLIENT] Event #4: [GOOG] Price = $184.50 at 15:30:02.000
[CLIENT] Event #5: [GOOG] Price = $185.10 at 15:30:02.500
[CLIENT] Limit reached (5 events). Initiating client disconnect...
[SERVER] Subscription closed. Connection context cancelled. Releasing resources for GOOG...
[DEMO] Shutting down background server...
[DEMO] Demonstration completed successfully.
```

### 2. Run the Server Separately
Start a persistent SUBSCRIBE server:
```bash
go run main.go -mode=server -addr=127.0.0.1:8080
```

### 3. Run the Client Separately
Connect a separate client to the server:
```bash
go run main.go -mode=client -addr=127.0.0.1:8080 -ticker=AAPL
```

### 4. Query using cURL
Because `SUBSCRIBE` is sent over standard HTTP protocols, you can test it with `curl` (using `-N` to disable buffering and `-X` to set the custom verb):
```bash
curl -N -X SUBSCRIBE http://127.0.0.1:8080/subscribe \
  -H "Content-Type: application/json" \
  -d '{"ticker": "MSFT", "interval_ms": 1000}'
```

---

## Running the Tests

To verify that the prototype logic passes all semantic HTTP checks and gracefully handles lifecycle teardowns:
```bash
go test -v ./...
```
