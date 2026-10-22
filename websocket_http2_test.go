package http_test

import (
	gohttp "apigo.cc/go/http"
	"bytes"
	"context"
	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebSocketHTTP2(t *testing.T) {
	requireExtendedConnect(t)
	server := httptest.NewServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.ProtoMajor != 2 || r.Header.Get(":protocol") != "websocket" || r.Header.Get("Sec-WebSocket-Key") != "" {
			t.Errorf("invalid RFC8441 request: %v", r)
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, Subprotocols: []string{"chat"}, EnableCompression: true}
		conn, err := gohttp.UpgradeWebSocket(&upgrader, w, r, http.Header{"Set-Cookie": {"one=1", "two=2"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if conn.WriteMessage(kind, data) != nil {
				return
			}
		}
	}), &http2.Server{}))
	defer server.Close()
	client := gohttp.NewClientH2C(time.Second)
	client.SetWebSocketHTTP2(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, response, err := client.OpenContext(ctx, "ws"+server.URL[4:]+"/socket", http.Header{"Sec-WebSocket-Protocol": {"chat"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if response.StatusCode != 200 || response.ProtoMajor != 2 || len(response.Header.Values("Set-Cookie")) != 2 || conn.Conn.Subprotocol() != "chat" {
		t.Fatalf("handshake: %+v", response)
	}
	response.Body.Close()
	for _, kind := range []int{websocket.TextMessage, websocket.BinaryMessage} {
		if err := conn.Conn.WriteMessage(kind, []byte("hello")); err != nil {
			t.Fatal(err)
		}
		got, data, err := conn.Conn.ReadMessage()
		if err != nil || got != kind || string(data) != "hello" {
			t.Fatalf("echo: %d %s %v", got, data, err)
		}
	}
	cancel()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.Conn.ReadMessage(); err == nil {
		t.Fatal("canceled connection readable")
	}
}
func TestWebSocketHTTP2Reject(t *testing.T) {
	requireExtendedConnect(t)
	server := httptest.NewServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1")
		http.Error(w, "denied", 403)
	}), &http2.Server{}))
	defer server.Close()
	client := gohttp.NewClientH2C(time.Second)
	client.SetWebSocketHTTP2(true)
	_, response, err := client.Open("ws" + server.URL[4:])
	if err == nil || response == nil || response.StatusCode != 403 {
		t.Fatalf("rejection: %v %v", response, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "denied\n" {
		t.Fatalf("body: %q", body)
	}
}

func requireExtendedConnect(t *testing.T) {
	t.Helper()
	if !strings.Contains(os.Getenv("GODEBUG"), "http2xconnect=1") {
		t.Skip("run with GODEBUG=http2xconnect=1 (read by x/net at startup)")
	}
}

func TestWebSocketHTTP2MultiplexAndTimeout(t *testing.T) {
	requireExtendedConnect(t)
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			<-r.Context().Done()
			return
		}
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := gohttp.UpgradeWebSocket(&upgrader, w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if conn.WriteMessage(kind, data) != nil {
				return
			}
		}
	}), &http2.Server{}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client := gohttp.NewClientH2C(time.Second)
	client.SetWebSocketHTTP2(true)
	first, _, err := client.Open("ws" + server.URL[4:])
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, _, err := client.Clone().Open("ws" + server.URL[4:])
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	if err = first.Send("survivor"); err != nil {
		t.Fatal(err)
	}
	if value, err := first.ReadString(); err != nil || value != "survivor" {
		t.Fatalf("surviving stream: %q %v", value, err)
	}
	if connections.Load() != 1 {
		t.Fatalf("connections: %d", connections.Load())
	}
	stalled := client.Clone()
	stalled.GetRawClient().Timeout = 20 * time.Millisecond
	if conn, _, err := stalled.Open("ws" + server.URL[4:] + "/stall"); err == nil {
		conn.Close()
		t.Fatal("stalled handshake succeeded")
	}
}

func BenchmarkWebSocketHTTP2Echo(b *testing.B) {
	if !strings.Contains(os.Getenv("GODEBUG"), "http2xconnect=1") {
		b.Skip("requires startup GODEBUG=http2xconnect=1")
	}
	server := httptest.NewServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := gohttp.UpgradeWebSocket(&upgrader, w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if conn.WriteMessage(kind, data) != nil {
				return
			}
		}
	}), &http2.Server{}))
	defer server.Close()
	client := gohttp.NewClientH2C(time.Second)
	client.SetWebSocketHTTP2(true)
	conn, _, err := client.Open("ws" + server.URL[4:])
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	payload := bytes.Repeat([]byte("x"), 1024)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := conn.Send(payload); err != nil {
			b.Fatal(err)
		}
		if _, err := conn.ReadBytes(); err != nil {
			b.Fatal(err)
		}
	}
}
