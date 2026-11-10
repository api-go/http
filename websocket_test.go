package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ah "github.com/api-go/http"
	"github.com/gorilla/websocket"
)

func TestWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Global") != "global" || r.Header.Get("X-Request") != "request" {
			t.Errorf("unexpected headers: %v", r.Header)
		}
		if cookie, err := r.Cookie("session"); err != nil || cookie.Value != "ready" {
			t.Errorf("unexpected cookie: %v, %v", cookie, err)
		}
		conn, err := upgrader.Upgrade(w, r, http.Header{"X-Handshake": {"ok"}})
		if err != nil {
			return
		}
		defer conn.Close()
		for i := 0; i < 3; i++ {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(messageType, data); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	client := ah.NewClient(time.Second)
	client.SetGlobalHeader("X-Global", "global")
	client.GetRawClient().Jar.SetCookies(mustURL(t, server.URL), []*http.Cookie{{Name: "session", Value: "ready"}})

	conn, _, err := client.Open("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"X-Request": {"request"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if conn.Response.StatusCode != http.StatusSwitchingProtocols || conn.Response.Header.Get("X-Handshake") != "ok" {
		t.Fatalf("unexpected handshake response: %v", conn.Response)
	}

	if err := conn.Send("text"); err != nil {
		t.Fatal(err)
	}
	if value, err := conn.ReadString(); err != nil || value != "text" {
		t.Fatalf("unexpected text response: %q, %v", value, err)
	}

	if err := conn.Send([]byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if value, err := conn.ReadBytes(); err != nil || string(value) != string([]byte{1, 2, 3}) {
		t.Fatalf("unexpected binary response: %v, %v", value, err)
	}

	want := map[string]any{"message": "json"}
	if err := conn.Send(want); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil || got["message"] != "json" {
		t.Fatalf("unexpected JSON response: %v, %v", got, err)
	}
}

func TestWebSocketContextCancellation(t *testing.T) {
	upgrader := websocket.Upgrader{}
	connected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		close(connected)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	conn, _, err := ah.OpenContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	<-connected
	cancel()
	if _, err := conn.ReadString(); err == nil {
		t.Fatal("read was not interrupted by context cancellation")
	}
}

func TestWebSocketHandshakeErrorRetainsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Reason", "denied")
		http.Error(w, "denied", http.StatusForbidden)
	}))
	defer server.Close()

	_, _, err := ah.Open("ws" + strings.TrimPrefix(server.URL, "http"))
	var handshakeErr *ah.WebSocketHandshakeError
	if !errors.As(err, &handshakeErr) {
		t.Fatalf("unexpected error: %v", err)
	}
	defer handshakeErr.Response.Body.Close()
	if handshakeErr.Response.StatusCode != http.StatusForbidden || handshakeErr.Response.Header.Get("X-Reason") != "denied" {
		t.Fatalf("unexpected handshake response: %v", handshakeErr.Response)
	}
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
