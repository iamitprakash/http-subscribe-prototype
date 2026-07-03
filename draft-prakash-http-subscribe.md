---
title: The HTTP SUBSCRIBE Method
abbrev: HTTP SUBSCRIBE
docname: draft-prakash-http-subscribe-00
date: 2026-07-04
category: std

ipr: trust200902
area: Applications and Real-Time (art)
workgroup: HTTP Working Group (httpbis)
keyword: HTTP, subscribe, push, server-sent-events, websockets
author:
  - ins: A. Prakash
    name: Amit Prakash
    org: Boeing
    email: amitbcet2k15@gmail.com

normative:
  RFC2119:
  RFC8174:
  RFC9110:

informative:
  RFC5789:
  RFC10008:
    title: "The HTTP QUERY Method"
    date: 2026-06
    author:
      - name: IETF
--- abstract

This document defines the HTTP `SUBSCRIBE` request method. The `SUBSCRIBE` method allows a client to establish a long-lived, safe connection to a resource to receive real-time updates and event streams. It enables servers to push data to clients using standard HTTP structures (such as HTTP/2 or HTTP/3 streams) while supporting a request body for subscription parameters, avoiding the protocol-switching overhead of WebSockets and the URL limitations of Server-Sent Events (SSE) via `GET`.

--- middle

# Introduction

Modern web applications require efficient, low-latency, and real-time communication channels from the server to the client. Examples include chat applications, stock market tickers, collaborative document editing, and live dashboards.

Historically, real-time push has been achieved using three primary workarounds:

1.  **Long Polling:** Recreating HTTP requests continuously. This is highly inefficient and creates substantial connection overhead.
2.  **WebSockets:** Upgrades the connection from HTTP to a separate bidirectional TCP-based protocol. While efficient, it bypasses HTTP intermediaries (like load balancers, reverse proxies, and Web Application Firewalls), breaks semantic caching, complicates authentication, and often struggles with strict enterprise firewalls.
3.  **Server-Sent Events (SSE):** Utilizes standard HTTP `GET` requests with `text/event-stream` responses. However, because `GET` requests do not support a request body, clients must pass subscription parameters (such as filter expressions, selected fields, or authentication tokens) within the URL query string. This leads to problems with URL length limits, logging of sensitive data, and overall architectural rigidity.

This specification introduces the `SUBSCRIBE` method to address these issues. `SUBSCRIBE` is a safe HTTP method that allows the client to send subscription parameters in the request body while establishing a long-lived, server-push event stream.

# Terminology

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 {{RFC2119}} {{RFC8174}} when, and only when, they appear in all capitals, as shown here.

# The SUBSCRIBE Method

The `SUBSCRIBE` method is used to request a persistent, real-time event stream from a target resource.

- **Safe:** Yes. A `SUBSCRIBE` request is read-only; it MUST NOT modify the state of the target resource on the server.
- **Idempotent:** Yes. Multiple identical `SUBSCRIBE` requests will yield identical event stream configurations.
- **Request Body:** Allowed. The request body contains subscription parameters (e.g., query filters, requested fields, backfill start time, or sub-topics).
- **Response Body:** Allowed. The response body is a long-lived stream of events or data updates.

## Request Semantics

Clients send a `SUBSCRIBE` request to the target URI. The request body SHOULD specify the parameters of the subscription. For example, a JSON payload may describe a topic filter:

~~~ http
SUBSCRIBE /events/trades HTTP/1.1
Host: api.example.com
Content-Type: application/json
Accept: text/event-stream

{
  "ticker": "GOOG",
  "min_volume": 100,
  "fields": ["price", "volume", "timestamp"]
}
~~~

## Response Semantics

A successful response is indicated by the `200 OK` status code. The response body MUST consist of a continuous stream of structured data chunks.

To maintain the subscription, the server holds the response stream open indefinitely.

- **HTTP/1.1:** The server MUST use chunked transfer encoding (`Transfer-Encoding: chunked`) to stream data.
- **HTTP/2 and HTTP/3:** The server streams data over a single multiplexed stream, utilizing native frame transport without requiring chunked transfer encoding.

The response `Content-Type` SHOULD indicate a streaming protocol, such as `text/event-stream` or `application/x-ndjson`.

Example response header and initial stream chunk:

~~~ http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache, no-store
Connection: keep-alive

data: {"price": 175.20, "volume": 150, "timestamp": 1719999999}

data: {"price": 175.25, "volume": 500, "timestamp": 1720000005}
~~~

# Client and Server Behavior

## Client Behavior

A client initiates a subscription by issuing a `SUBSCRIBE` request. The client MUST be prepared to read the response body incrementally as data chunks arrive.

If the client wishes to terminate the subscription, it MUST close the transport-level stream (or connection).

## Server Behavior

Upon receiving a `SUBSCRIBE` request, the server:

1.  MUST parse and validate the request body and headers.
2.  MUST verify authorization for the requested subscription.
3.  On success, MUST send response headers (e.g., `200 OK`) and keep the stream active.
4.  MUST push events to the stream as they occur.
5.  SHOULD periodically transmit keep-alive/heartbeat chunks (such as empty comments in `text/event-stream`) to prevent intermediary timeouts.

If the request is invalid or unauthorized, the server MUST return an appropriate 4xx or 5xx status code and close the connection immediately.

# Caching Considerations

Because `SUBSCRIBE` responses represent dynamic, real-time streams of events, they MUST NOT be cached by shared caches or HTTP intermediaries.

Servers MUST include a `Cache-Control: no-cache, no-store` header in the response. Intermediaries MUST immediately forward both the request and response stream without buffering.

# Security Considerations

## Connection Exhaustion (DoS)

Long-lived connections consume file descriptors and memory. Malicious clients could open thousands of subscriptions to deplete server resources (Denial of Service).

Servers SHOULD:

- Enforce limits on the number of concurrent subscriptions per client/IP address.
- Support HTTP/2 and HTTP/3 to multiplex subscriptions over a minimal number of TCP/QUIC connections.
- Implement aggressive timeouts for clients that do not read stream chunks.

## Authentication and Authorization

Subscriptions often contain sensitive live data. Because connections are long-lived, token expiration (e.g., OAuth tokens) must be handled. Servers SHOULD validate token longevity during the initial request handshake. If a token expires while a stream is active, the server MAY push an expiration event and close the connection, forcing the client to re-authenticate and re-subscribe.

# IANA Considerations

IANA is requested to register the `SUBSCRIBE` method in the "HTTP Method Registry" under the Hypertext Transfer Protocol (HTTP) Parameters registry:

- **Method Name:** `SUBSCRIBE`
- **Safe:** Yes
- **Idempotent:** Yes
- **Reference:** This document
