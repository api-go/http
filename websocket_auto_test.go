package http_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gohttp "github.com/api-go/http"
	"github.com/gorilla/websocket"
)

func autoEcho(w http.ResponseWriter, r *http.Request) {
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
}
func TestWebSocketAutoTLS(t *testing.T) {
	requireExtendedConnect(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(autoEcho))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := gohttp.NewClient(time.Second)
	client.GetRawClient().Transport = server.Client().Transport
	client.SetWebSocketOptions(true, nil)
	defer client.Destroy()
	conn, response, err := client.Open("wss" + server.URL[5:])
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if response.ProtoMajor != 2 {
		t.Fatalf("protocol: %s", response.Proto)
	}
	conn.Send("auto")
	if value, err := conn.ReadString(); err != nil || value != "auto" {
		t.Fatalf("echo: %q %v", value, err)
	}
}
func TestWebSocketAutoTLSFallbackAndCache(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(autoEcho))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	defer server.Close()
	client := gohttp.NewClient(time.Second)
	client.GetRawClient().Transport = server.Client().Transport
	defer client.Destroy()
	for i := 0; i < 2; i++ {
		conn, response, err := client.Clone().Open("wss" + server.URL[5:])
		if err != nil {
			t.Fatal(err)
		}
		if response.ProtoMajor != 1 {
			t.Fatalf("protocol: %s", response.Proto)
		}
		conn.Send("fallback")
		if value, err := conn.ReadString(); err != nil || value != "fallback" {
			t.Fatalf("echo: %q %v", value, err)
		}
		conn.Close()
	}
	if connections.Load() != 3 {
		t.Fatalf("expected one H2 negotiation plus two H1 connections, got %d", connections.Load())
	}
}
func TestWebSocketAutoDoesNotRetryRejections(t *testing.T) {
	requireExtendedConnect(t)
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "denied")
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := gohttp.NewClient(time.Second)
	client.GetRawClient().Transport = server.Client().Transport
	defer client.Destroy()
	_, response, err := client.Open("wss" + server.URL[5:])
	if err == nil || response == nil || response.StatusCode != 401 || response.ProtoMajor != 2 {
		t.Fatalf("rejection: %v %+v", err, response)
	}
	response.Body.Close()
	if requests.Load() != 1 {
		t.Fatalf("requests: %d", requests.Load())
	}
}
func TestWebSocketAutoPlainAndInvalidCertificate(t *testing.T) {
	var requests atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); autoEcho(w, r) })
	plain := httptest.NewServer(handler)
	defer plain.Close()
	client := gohttp.NewClient(time.Second)
	defer client.Destroy()
	conn, response, err := client.Open("ws" + plain.URL[4:])
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if response.ProtoMajor != 1 {
		t.Fatalf("plain protocol: %s", response.Proto)
	}
	secured := httptest.NewUnstartedServer(handler)
	secured.EnableHTTP2 = true
	secured.StartTLS()
	defer secured.Close()
	_, _, err = client.Open("wss" + secured.URL[5:])
	if err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	if requests.Load() != 1 {
		t.Fatalf("certificate failure sent a request: %d", requests.Load())
	}
}
func TestWebSocketAutoRetainsCustomDialer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(autoEcho))
	defer server.Close()
	client := gohttp.NewClient(time.Second)
	defer client.Destroy()
	var dials atomic.Int32
	client.SetWebSocketDialer(&websocket.Dialer{NetDialTLSContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		return (&tls.Dialer{Config: server.Client().Transport.(*http.Transport).TLSClientConfig}).DialContext(ctx, network, address)
	}})
	conn, response, err := client.Open("wss" + server.URL[5:])
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if response.ProtoMajor != 1 || dials.Load() != 1 {
		t.Fatalf("custom dialer: %s %d", response.Proto, dials.Load())
	}
}

// Run the no-RFC8441 peer in a separate process: Go reads its capability switch
// at initialization, so changing GODEBUG inside a running test would be invalid.
func TestWebSocketAutoSettingsFallback(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestWebSocketNoExtendedConnectPeer$")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GODEBUG=") && !strings.HasPrefix(entry, "GO_HTTP_WS_PEER=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "GODEBUG=http2xconnect=0", "GO_HTTP_WS_PEER=1")
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		input.Close()
		if err := command.Wait(); err != nil {
			t.Errorf("peer: %v %s", err, stderr.String())
		}
	}()
	address, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var dials atomic.Int32
	client := gohttp.NewClient(time.Second)
	defer client.Destroy()
	client.GetRawClient().Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	for i := 0; i < 2; i++ {
		conn, response, err := client.Clone().Open(strings.TrimSpace(address))
		if err != nil {
			t.Fatal(err)
		}
		if response.ProtoMajor != 1 {
			t.Fatal(response.Proto)
		}
		conn.Send("old-server")
		if value, err := conn.ReadString(); err != nil || value != "old-server" {
			t.Fatalf("echo: %q %v", value, err)
		}
		conn.Close()
	}
	if dials.Load() != 3 {
		t.Fatalf("expected one SETTINGS probe and two H1 connections; got %d dials", dials.Load())
	}
}
func TestWebSocketNoExtendedConnectPeer(t *testing.T) {
	if os.Getenv("GO_HTTP_WS_PEER") != "1" {
		t.Skip("subprocess fixture")
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 1 || r.Method != "GET" {
			http.Error(w, "unexpected CONNECT reached backend", 401)
			return
		}
		autoEcho(w, r)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	fmt.Println("wss" + server.URL[5:])
	io.Copy(io.Discard, os.Stdin)
}
