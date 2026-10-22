package http

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
)

// IsWebSocketRequest recognizes RFC 6455 upgrades and RFC 8441 extended CONNECT.
func IsWebSocketRequest(request *http.Request) bool {
	return websocket.IsWebSocketUpgrade(request) || request.ProtoMajor == 2 && request.Method == http.MethodConnect && request.Header.Get(":protocol") == "websocket"
}

// UpgradeWebSocket retains Gorilla framing and options over an HTTP/2 stream.
// The handler must remain running until the connection is closed. Servers using
// x/net/http2 must start with GODEBUG=http2xconnect=1 to advertise RFC 8441.
func UpgradeWebSocket(upgrader *websocket.Upgrader, writer http.ResponseWriter, request *http.Request, headers http.Header) (*websocket.Conn, error) {
	if request.ProtoMajor != 2 {
		return upgrader.Upgrade(writer, request, headers)
	}
	if !IsWebSocketRequest(request) || request.Header.Get("Sec-WebSocket-Key") != "" || request.Header.Get("Sec-WebSocket-Accept") != "" {
		http.Error(writer, "invalid extended WebSocket CONNECT", http.StatusBadRequest)
		return nil, errors.New("websocket: invalid extended CONNECT")
	}
	cloned := request.Clone(request.Context())
	cloned.Method = http.MethodGet
	cloned.Header.Del(":protocol")
	cloned.Header.Set("Connection", "Upgrade")
	cloned.Header.Set("Upgrade", "websocket")
	// Only the local Gorilla handshake uses a key; RFC 8441 carries no key/accept.
	cloned.Header.Set("Sec-WebSocket-Key", "AAAAAAAAAAAAAAAAAAAAAA==")
	stream := &webSocketServerStream{writer: writer, body: request.Body, controller: http.NewResponseController(writer)}
	return upgrader.Upgrade(&webSocketStreamWriter{ResponseWriter: writer, stream: stream}, cloned, headers)
}

type webSocketStreamWriter struct {
	http.ResponseWriter
	stream *webSocketServerStream
}

func (writer *webSocketStreamWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return writer.stream, bufio.NewReadWriter(bufio.NewReader(writer.stream), bufio.NewWriter(writer.stream)), nil
}

type webSocketServerStream struct {
	writer     http.ResponseWriter
	body       io.ReadCloser
	controller *http.ResponseController
	handshake  bool
	closed     atomic.Bool
}

func (stream *webSocketServerStream) Read(data []byte) (int, error) {
	if stream.closed.Load() {
		return 0, net.ErrClosed
	}
	return stream.body.Read(data)
}
func (stream *webSocketServerStream) Write(data []byte) (int, error) {
	if stream.closed.Load() {
		return 0, net.ErrClosed
	}
	if !stream.handshake {
		response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
		if err != nil {
			return 0, err
		}
		for key, values := range response.Header {
			switch http.CanonicalHeaderKey(key) {
			case "Connection", "Upgrade", "Sec-Websocket-Accept":
				continue
			}
			stream.writer.Header()[key] = values
		}
		stream.writer.Header().Del("Content-Length")
		stream.writer.WriteHeader(http.StatusOK)
		stream.handshake = true
		if err := stream.controller.Flush(); err != nil {
			return 0, err
		}
		return len(data), nil
	}
	n, err := stream.writer.Write(data)
	if err == nil {
		err = stream.controller.Flush()
	}
	return n, err
}
func (stream *webSocketServerStream) Close() error {
	if stream.closed.Swap(true) {
		return nil
	}
	// HTTP/2 deadlines apply to this stream, never to the shared TCP connection.
	_ = stream.controller.SetWriteDeadline(time.Now())
	return stream.body.Close()
}
func (stream *webSocketServerStream) LocalAddr() net.Addr { return webSocketStreamAddr("http2-local") }
func (stream *webSocketServerStream) RemoteAddr() net.Addr {
	return webSocketStreamAddr("http2-remote")
}
func (stream *webSocketServerStream) SetDeadline(deadline time.Time) error {
	return errors.Join(stream.SetReadDeadline(deadline), stream.SetWriteDeadline(deadline))
}
func (stream *webSocketServerStream) SetReadDeadline(deadline time.Time) error {
	return stream.controller.SetReadDeadline(deadline)
}
func (stream *webSocketServerStream) SetWriteDeadline(deadline time.Time) error {
	return stream.controller.SetWriteDeadline(deadline)
}

type webSocketStreamAddr string

func (address webSocketStreamAddr) Network() string { return "http2" }
func (address webSocketStreamAddr) String() string  { return string(address) }

// SetWebSocketHTTP2 overrides automatic negotiation: true requires RFC 8441
// without fallback, and false forces HTTP/1.1.
// Configure before use. The HTTP/2 transport is shared by client clones.
func (client *Client) SetWebSocketHTTP2(enabled bool) {
	client.websocketMu.Lock()
	defer client.websocketMu.Unlock()
	client.websocketHTTP2 = enabled
	client.websocketHTTP2Configured = true
	if enabled {
		client.prepareWebSocketHTTP2Locked()
	}
}

func (client *Client) prepareWebSocketHTTP2() {
	client.websocketMu.Lock()
	defer client.websocketMu.Unlock()
	client.prepareWebSocketHTTP2Locked()
}

func (client *Client) prepareWebSocketHTTP2Locked() {
	if client.pool == nil {
		return
	}
	if client.websocketHTTP2Fallback == nil {
		client.websocketHTTP2Fallback = &webSocketFallbackCache{origins: make(map[string]time.Time)}
	}
	if client.websocketHTTP2Transport != nil {
		return
	}
	if transport, ok := client.pool.Transport.(*http2.Transport); ok {
		client.websocketHTTP2Transport = transport
		return
	}
	transport := &http2.Transport{IdleConnTimeout: 90 * time.Second}
	var dialContext func(context.Context, string, string) (net.Conn, error)
	if native, ok := client.pool.Transport.(*http.Transport); ok {
		if native.IdleConnTimeout > 0 {
			transport.IdleConnTimeout = native.IdleConnTimeout
		}
		if native.TLSClientConfig != nil {
			transport.TLSClientConfig = native.TLSClientConfig.Clone()
		}
		dialContext = native.DialContext
	}
	transport.DialTLSContext = func(ctx context.Context, network, address string, config *tls.Config) (net.Conn, error) {
		config = config.Clone()
		config.NextProtos = []string{"h2", "http/1.1"}
		var plain net.Conn
		var err error
		if dialContext != nil {
			plain, err = dialContext(ctx, network, address)
		} else {
			plain, err = (&net.Dialer{}).DialContext(ctx, network, address)
		}
		if err != nil {
			return nil, err
		}
		secured := tls.Client(plain, config)
		if err := secured.HandshakeContext(ctx); err != nil {
			plain.Close()
			return nil, err
		}
		if secured.ConnectionState().NegotiatedProtocol != "h2" {
			secured.Close()
			return nil, errWebSocketHTTP2Unavailable
		}
		return secured, nil
	}
	client.websocketHTTP2Transport = transport
}

func (client *Client) openWebSocketHTTP2(ctx context.Context, target string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, nil, err
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return nil, nil, errors.New("websocket: expected ws or wss URL")
	}
	// Do not silently bypass an HTTP proxy or a custom TLS dialer when deriving
	// a transport. Explicit *http2.Transport configurations own these policies.
	if native, ok := client.pool.Transport.(*http.Transport); ok {
		if native.DialTLSContext != nil || native.DialTLS != nil {
			return nil, nil, errors.New("websocket: HTTP/2 requires an explicit http2.Transport for custom TLS dialing")
		}
		if native.Proxy != nil {
			proxy, err := native.Proxy(&http.Request{URL: parsed, Header: make(http.Header)})
			if err != nil {
				return nil, nil, err
			}
			if proxy != nil {
				return nil, nil, errors.New("websocket: HTTP/2 requires an explicit http2.Transport for HTTP proxy dialing")
			}
		}
	}
	if client.pool.Transport != nil {
		switch client.pool.Transport.(type) {
		case *http.Transport, *http2.Transport:
		default:
			return nil, nil, errors.New("websocket: configure an explicit http2.Transport for HTTP/2 dialing")
		}
	}
	cloned := client.Clone()
	cloned.SetWebSocketHTTP2(false)
	client.websocketMu.RLock()
	transport := client.websocketHTTP2Transport
	client.websocketMu.RUnlock()
	dialer := cloned.WebSocketDialer()
	dialer.Proxy = nil
	type handshakeResult struct {
		response *http.Response
		err      error
	}
	results := make(chan handshakeResult, 1)
	dial := func(_ context.Context, _, _ string) (net.Conn, error) {
		local, remote := net.Pipe()
		streamCtx, cancel := context.WithCancel(ctx)
		go func() {
			defer remote.Close()
			defer cancel()
			request, readErr := http.ReadRequest(bufio.NewReader(remote))
			if readErr != nil {
				results <- handshakeResult{err: readErr}
				return
			}
			body, outgoing := io.Pipe()
			defer body.Close()
			defer outgoing.Close()
			connect := request.Clone(streamCtx)
			connect.Method = http.MethodConnect
			connect.URL = parsed
			connect.RequestURI = ""
			connect.Body = body
			connect.ContentLength = -1
			key := connect.Header.Get("Sec-WebSocket-Key")
			for _, field := range []string{"Connection", "Upgrade", "Sec-WebSocket-Key", "Sec-WebSocket-Accept"} {
				connect.Header.Del(field)
			}
			connect.Header.Set(":protocol", "websocket")
			stop := context.AfterFunc(streamCtx, func() { remote.Close(); body.Close(); outgoing.Close() })
			defer stop()
			response, roundErr := transport.RoundTrip(connect)
			results <- handshakeResult{response: response, err: roundErr}
			if roundErr != nil {
				return
			}
			if response.StatusCode != http.StatusOK {
				// Gorilla retains only a bounded rejection body.
				defer response.Body.Close()
				rejection := *response
				retained, _ := io.ReadAll(io.LimitReader(response.Body, 1025))
				rejection.Body = io.NopCloser(bytes.NewReader(retained))
				rejection.ContentLength = int64(len(retained))
				rejection.TransferEncoding = nil
				_ = rejection.Write(remote)
				return
			}
			defer response.Body.Close()
			accepted := response.Header.Clone()
			accepted.Set("Connection", "Upgrade")
			accepted.Set("Upgrade", "websocket")
			digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
			accepted.Set("Sec-WebSocket-Accept", base64.StdEncoding.EncodeToString(digest[:]))
			accepted.Del("Content-Length")
			_, _ = io.WriteString(remote, "HTTP/1.1 101 Switching Protocols\r\n")
			_ = accepted.Write(remote)
			_, _ = io.WriteString(remote, "\r\n")
			copied := make(chan struct{})
			go func() { defer close(copied); _, _ = io.Copy(outgoing, remote); _ = outgoing.Close() }()
			_, _ = io.Copy(remote, response.Body)
			_ = remote.Close()
			_ = outgoing.Close()
			<-copied
		}()
		return &webSocketClientPipe{Conn: local, cancel: cancel}, nil
	}
	dialer.NetDialContext = dial
	dialer.NetDialTLSContext = dial
	cloned.SetWebSocketDialer(dialer)
	conn, response, openErr := cloned.OpenContext(ctx, target, headers...)
	if openErr != nil {
		select {
		case result := <-results:
			if result.err != nil {
				return nil, nil, result.err
			}
			if response != nil && result.response != nil {
				response.Proto = result.response.Proto
				response.ProtoMajor = result.response.ProtoMajor
				response.ProtoMinor = result.response.ProtoMinor
			}
		default:
		}
		return conn, response, openErr
	}
	actual := (<-results).response
	// The caller owns only the logical WebSocket. Closing handshake.Body must not
	// close the live HTTP/2 stream, matching Gorilla's HTTP/1.1 response contract.
	published := *actual
	published.Body = io.NopCloser(strings.NewReader(""))
	conn.Response = &published
	return conn, &published, nil
}

type webSocketClientPipe struct {
	net.Conn
	cancel context.CancelFunc
}

func (conn *webSocketClientPipe) Close() error { conn.cancel(); return conn.Conn.Close() }
