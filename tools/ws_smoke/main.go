// Package main provides a WebSocket smoke check used by `scripts/smoke_core.sh`.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const graphqlTransportWSSubprotocol = "graphql-transport-ws"

type options struct {
	BaseURL        string
	Token          string
	TimeoutSeconds int
	Insecure       bool
}

type wsEnvelope struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func main() {
	var opts options
	flag.StringVar(&opts.BaseURL, "base-url", os.Getenv("SMOKE_BASE_URL"), "base url (env: SMOKE_BASE_URL)")
	flag.StringVar(&opts.Token, "token", os.Getenv("SMOKE_TOKEN"), "auth token (env: SMOKE_TOKEN)")
	flag.IntVar(&opts.TimeoutSeconds, "timeout-seconds", 15, "timeout seconds (env: SMOKE_TIMEOUT_SECONDS)")
	flag.BoolVar(&opts.Insecure, "insecure", os.Getenv("SMOKE_INSECURE") == "1", "allow insecure TLS")
	flag.Parse()

	opts.BaseURL = strings.TrimSpace(opts.BaseURL)
	if opts.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "error: --base-url is required")
		os.Exit(2)
	}
	if opts.TimeoutSeconds <= 0 {
		fmt.Fprintln(os.Stderr, "error: --timeout-seconds must be positive")
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(opts options) error {
	base, err := url.Parse(opts.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid --base-url %q: %w", opts.BaseURL, err)
	}
	if strings.TrimSpace(base.Host) == "" {
		return fmt.Errorf("invalid --base-url %q: missing host", opts.BaseURL)
	}

	wsScheme := "wss"
	if strings.EqualFold(base.Scheme, "http") {
		wsScheme = "ws"
	}

	wsHost := deriveWSHost(base.Host)
	graphqlURL := (&url.URL{Scheme: wsScheme, Host: wsHost, Path: "/"}).String()

	streamingURL := &url.URL{Scheme: wsScheme, Host: wsHost, Path: "/stream"}
	accessToken := stripBearerPrefix(opts.Token)
	if accessToken != "" {
		q := streamingURL.Query()
		q.Set("access_token", accessToken)
		streamingURL.RawQuery = q.Encode()
	}

	timeout := time.Duration(opts.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Println("=== Smoke WebSockets ===")
	fmt.Println("GraphQL:", graphqlURL)
	fmt.Println("Stream: ", streamingURL.String())
	fmt.Println()

	if err := checkGraphQLWS(ctx, graphqlURL, accessToken, opts.Insecure); err != nil {
		return err
	}
	if err := checkStreamingWS(ctx, streamingURL.String(), opts.Insecure); err != nil {
		return err
	}

	fmt.Println("✓ smoke-websockets passed")
	return nil
}

func deriveWSHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}

	name := host
	port := ""
	if h, p, splitErr := net.SplitHostPort(host); splitErr == nil {
		name = h
		port = p
	}

	nameLower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(nameLower, "ws."):
		// already ws.*
	case strings.HasPrefix(nameLower, "api."):
		name = "ws." + strings.TrimPrefix(name, "api.")
	default:
		name = "ws." + name
	}

	if port == "" {
		return name
	}
	return net.JoinHostPort(name, port)
}

func stripBearerPrefix(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) > 7 && strings.EqualFold(token[:7], "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	return strings.TrimSpace(token)
}

func checkGraphQLWS(ctx context.Context, urlStr string, accessToken string, insecure bool) error {
	fmt.Println("1) GraphQL WS connect + ping/pong")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     []string{graphqlTransportWSSubprotocol},
		TLSClientConfig:  tlsConfig(insecure),
	}

	conn, resp, err := dialer.DialContext(ctx, urlStr, nil)
	if err != nil {
		return fmt.Errorf("graphql ws dial failed: %w (http=%s)", err, httpStatus(resp))
	}
	defer func() { _ = conn.Close() }()

	if got := conn.Subprotocol(); got != graphqlTransportWSSubprotocol {
		return fmt.Errorf("graphql ws subprotocol mismatch: got %q want %q", got, graphqlTransportWSSubprotocol)
	}

	if accessToken != "" {
		if err := graphqlConnectionInit(ctx, conn, accessToken); err != nil {
			return err
		}
		if err := graphqlSubscribe(ctx, conn); err != nil {
			return err
		}
	} else {
		fmt.Println("  - auth: skipped (no token)")
	}

	if err := graphqlPingPong(ctx, conn); err != nil {
		return err
	}

	fmt.Println("  ✓ ok")
	fmt.Println()
	return nil
}

func graphqlConnectionInit(ctx context.Context, conn *websocket.Conn, accessToken string) error {
	payload := map[string]any{
		"Authorization": "Bearer " + accessToken,
	}
	initMsg := map[string]any{
		"type":    "connection_init",
		"payload": payload,
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		return fmt.Errorf("graphql ws connection_init write failed: %w", err)
	}
	if err := waitForMessageType(ctx, conn, "connection_ack", 5*time.Second); err != nil {
		return fmt.Errorf("graphql ws connection_init failed: %w", err)
	}
	return nil
}

func graphqlSubscribe(ctx context.Context, conn *websocket.Conn) error {
	subMsg := map[string]any{
		"id":   "smoke1",
		"type": "subscribe",
		"payload": map[string]any{
			"query": "subscription { conversationUpdates { __typename } }",
		},
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("graphql ws subscribe write failed: %w", err)
	}

	// If the subscription is invalid/unauthorized, the server should emit an "error" for this id quickly.
	if err := failOnOperationError(ctx, conn, "smoke1", 1500*time.Millisecond); err != nil {
		return err
	}
	return nil
}

func graphqlPingPong(ctx context.Context, conn *websocket.Conn) error {
	if err := conn.WriteJSON(map[string]any{"type": "ping"}); err != nil {
		return fmt.Errorf("graphql ws ping write failed: %w", err)
	}
	if err := waitForMessageType(ctx, conn, "pong", 5*time.Second); err != nil {
		return fmt.Errorf("graphql ws pong not received: %w", err)
	}
	return nil
}

func checkStreamingWS(ctx context.Context, urlStr string, insecure bool) error {
	fmt.Println("2) Streaming WS connect + ping/pong")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  tlsConfig(insecure),
	}

	conn, resp, err := dialer.DialContext(ctx, urlStr, nil)
	if err != nil {
		return fmt.Errorf("streaming ws dial failed: %w (http=%s)", err, httpStatus(resp))
	}
	defer func() { _ = conn.Close() }()

	if err := conn.WriteJSON(map[string]any{"type": "ping"}); err != nil {
		return fmt.Errorf("streaming ws ping write failed: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		var msg struct {
			Type string `json:"type"`
		}
		if err := readJSONUntil(ctx, conn, deadline, &msg); err != nil {
			return fmt.Errorf("streaming ws read failed: %w", err)
		}
		if msg.Type == "pong" {
			break
		}
	}

	fmt.Println("  ✓ ok")
	fmt.Println()
	return nil
}

func tlsConfig(insecure bool) *tls.Config {
	if !insecure {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- smoke tooling supports insecure deployments
}

func waitForMessageType(ctx context.Context, conn *websocket.Conn, wantType string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var env wsEnvelope
		if err := readJSONUntil(ctx, conn, deadline, &env); err != nil {
			return err
		}
		switch env.Type {
		case wantType:
			return nil
		case "error":
			return fmt.Errorf("server error: %s", strings.TrimSpace(string(env.Payload)))
		case "connection_error":
			return fmt.Errorf("connection error: %s", strings.TrimSpace(string(env.Payload)))
		default:
			// ignore and keep reading until deadline
		}
	}
}

func failOnOperationError(ctx context.Context, conn *websocket.Conn, opID string, window time.Duration) error {
	deadline := time.Now().Add(window)
	for {
		var env wsEnvelope
		if err := readJSONUntil(ctx, conn, deadline, &env); err != nil {
			if isTimeout(err) {
				return nil
			}
			return err
		}
		if env.Type == "error" && env.ID == opID {
			return fmt.Errorf("graphql ws subscription rejected: %s", strings.TrimSpace(string(env.Payload)))
		}
	}
}

func readJSONUntil(ctx context.Context, conn *websocket.Conn, deadline time.Time, out any) error {
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("invalid json message: %w", err)
	}
	return nil
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

func httpStatus(resp *http.Response) string {
	if resp == nil {
		return "n/a"
	}
	return resp.Status
}
