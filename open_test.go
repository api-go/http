package http_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gohttp "apigo.cc/go/http"
	"github.com/gorilla/websocket"
)

func TestOpenRetainsMultiValueHeadersAndCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Values("X-Labels"); len(got) != 2 || got[0] != "one" || got[1] != "two" {
			t.Errorf("labels: %v", got)
		}
		if len(r.Cookies()) != 2 {
			t.Errorf("cookies: %v", r.Cookies())
		}
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, http.Header{"Set-Cookie": {"one=1; Path=/", "two=2; Path=/"}})
		if err == nil {
			conn.Close()
		}
	}))
	defer server.Close()
	conn, response, err := gohttp.Open("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"X-Labels": {"one", "two"}, "Cookie": {"a=1", "b=2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if len(response.Header.Values("Set-Cookie")) != 2 {
		t.Fatal(response.Header)
	}
}

func TestOpenRetainsHTTP2TransportTLSSettings(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			if r.ProtoMajor != 2 {
				t.Errorf("HTTP protocol: %s", r.Proto)
			}
			_, _ = io.WriteString(w, "h2")
			return
		}
		if r.ProtoMajor != 1 {
			t.Errorf("WebSocket protocol: %s", r.Proto)
		}
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err == nil {
			conn.Close()
		}
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := gohttp.NewClient(time.Second)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.NextProtos = []string{"h2", "http/1.1"}
	client.GetRawClient().Transport = transport
	client.SetWebSocketHTTP2(false)
	if result := client.Get(server.URL); result.Error != nil || result.Response.ProtoMajor != 2 {
		t.Fatalf("HTTP2: %v", result.Error)
	}
	conn, response, err := client.Open("wss" + strings.TrimPrefix(server.URL, "https"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if response.ProtoMajor != 1 || transport.TLSClientConfig.NextProtos[0] != "h2" {
		t.Fatal("TLS transport mutated")
	}
}

func TestOpenNormalizesTruncatedRejectionBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 4096)) }))
	defer server.Close()
	conn, response, err := gohttp.Open("ws" + strings.TrimPrefix(server.URL, "http"))
	var handshakeError *gohttp.WebSocketHandshakeError
	if conn != nil || !errors.As(err, &handshakeError) || response == nil {
		t.Fatalf("handshake: %v %v", response, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if len(body) != 1024 || response.ContentLength != 1024 || response.Header.Get("Content-Length") != "1024" || !handshakeError.BodyTruncated {
		t.Fatalf("body length: %d, response: %v", len(body), response)
	}
}

func TestOpenRejectsExpiredHandshake(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	client := gohttp.NewClient(25 * time.Millisecond)
	if conn, _, err := client.Open("ws" + strings.TrimPrefix(server.URL, "http")); err == nil {
		conn.Close()
		t.Fatal("handshake timeout ignored")
	}
}

func TestOpenRejectsUnofferedSubprotocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-WebSocket-Protocol": {"unoffered"}})
		if err == nil {
			conn.Close()
		}
	}))
	defer server.Close()
	conn, response, err := gohttp.Open("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"Sec-WebSocket-Protocol": {"chat"}})
	if err == nil || conn != nil || response == nil {
		t.Fatalf("handshake %v %v", response, err)
	}
	response.Body.Close()
}

func TestOpenCancelsPendingHandshake(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cancel(); <-r.Context().Done() }))
	defer server.Close()
	client := gohttp.NewClient(3 * time.Second)
	started := time.Now()
	_, _, err := client.OpenContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation waited for handshake timeout")
	}
}
