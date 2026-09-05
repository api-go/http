package http_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ah "apigo.cc/go/http"
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

	conn, err := client.WebSocket("ws"+strings.TrimPrefix(server.URL, "http"), "X-Request", "request")
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

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
