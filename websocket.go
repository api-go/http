package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"apigo.cc/go/cast"
	"github.com/gorilla/websocket"
)

// WebSocketConn wraps a WebSocket connection with the common message helpers.
type WebSocketConn struct {
	Conn     *websocket.Conn
	Response *http.Response
	readMu   sync.Mutex
	writeMu  sync.Mutex
	closeMu  sync.Once
	closed   chan struct{}
}

// WebSocketHandshakeError retains the HTTP response returned by a failed
// WebSocket upgrade. Callers inspecting Response must close its body.
type WebSocketHandshakeError struct {
	Response      *http.Response
	Err           error
	BodyTruncated bool
}

func (err *WebSocketHandshakeError) Error() string { return err.Err.Error() }
func (err *WebSocketHandshakeError) Unwrap() error { return err.Err }

// Open establishes an HTTP/1.1 WebSocket connection and returns its handshake.
func Open(url string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	return DefaultClient.OpenContext(context.Background(), url, headers...)
}

// OpenContext connects with the default client and closes the connection
// when ctx is cancelled.
func OpenContext(ctx context.Context, url string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	return DefaultClient.OpenContext(ctx, url, headers...)
}

// Open connects to a WebSocket server using the client's timeout,
// cookies, and global headers.
func (client *Client) Open(url string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	return client.OpenContext(context.Background(), url, headers...)
}

// OpenContext connects to a WebSocket server using the client's timeout,
// cookies, and global headers. Cancelling ctx closes an established connection.
func (client *Client) OpenContext(ctx context.Context, url string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestHeaders := make(http.Header)
	for _, fields := range headers {
		for key, values := range fields {
			for _, value := range values {
				requestHeaders.Add(key, value)
			}
		}
	}

	client.headersMu.RLock()
	for key, value := range client.globalHeaders {
		requestHeaders.Set(key, value)
	}
	client.headersMu.RUnlock()

	timeout := time.Duration(0)
	if client.pool != nil {
		timeout = client.pool.Timeout
	}

	dialer := client.WebSocketDialer()
	protocols := append([]string(nil), dialer.Subprotocols...)
	if values := requestHeaders.Values("Sec-WebSocket-Protocol"); len(values) > 0 {
		protocols = nil
		for _, value := range values {
			for _, protocol := range strings.Split(value, ",") {
				protocols = append(protocols, strings.TrimSpace(protocol))
			}
		}
		dialer.Subprotocols = nil
	}
	if timeout > 0 && (dialer.HandshakeTimeout <= 0 || timeout < dialer.HandshakeTimeout) {
		dialer.HandshakeTimeout = timeout
	}
	// Gorilla uses Context for dialing, but a cancellation after TCP connect
	// does not interrupt its handshake reads. Monitor the socket until the
	// handshake returns, then let the established connection's watcher take over.
	handshakeDone := make(chan struct{})
	monitor := func(conn net.Conn, err error) (net.Conn, error) {
		if err == nil && ctx.Done() != nil {
			go func() {
				select {
				case <-ctx.Done():
					_ = conn.Close()
				case <-handshakeDone:
				}
			}()
		}
		return conn, err
	}
	if dialer.NetDialTLSContext != nil {
		dialTLS := dialer.NetDialTLSContext
		dialer.NetDialTLSContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
			return monitor(dialTLS(dialCtx, network, address))
		}
	} else {
		dialContext, dial := dialer.NetDialContext, dialer.NetDial
		dialer.NetDialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
			if dialContext != nil {
				return monitor(dialContext(dialCtx, network, address))
			}
			if dial != nil {
				return monitor(dial(network, address))
			}
			return monitor((&net.Dialer{}).DialContext(dialCtx, network, address))
		}
	}
	conn, response, err := dialer.DialContext(ctx, url, requestHeaders)
	close(handshakeDone)
	if ctx.Err() != nil {
		if conn != nil {
			_ = conn.Close()
		}
		err = ctx.Err()
	}
	if err == nil && conn.Subprotocol() != "" {
		selected := conn.Subprotocol()
		valid := false
		for _, offered := range protocols {
			if selected == offered {
				valid = true
				break
			}
		}
		if !valid {
			_ = conn.Close()
			err = fmt.Errorf("websocket: upstream selected unoffered subprotocol %q", selected)
		}
	}
	if err != nil {
		if response != nil {
			truncated := normalizeWebSocketHandshake(response)
			return nil, response, &WebSocketHandshakeError{Response: response, Err: err, BodyTruncated: truncated}
		}
		return nil, nil, err
	}
	wrapped := &WebSocketConn{Conn: conn, Response: response, closed: make(chan struct{})}
	if done := ctx.Done(); done != nil {
		go func() {
			select {
			case <-done:
				_ = wrapped.Close()
			case <-wrapped.closed:
			}
		}()
	}
	return wrapped, response, nil
}

// SetWebSocketDialer installs an independent configuration snapshot. nil restores
// defaults derived from the HTTP transport. Configure clients before use.
func (client *Client) SetWebSocketDialer(dialer *websocket.Dialer) {
	client.websocketMu.Lock()
	defer client.websocketMu.Unlock()
	client.websocketDialer = cloneWebSocketDialer(dialer)
}

func cloneWebSocketDialer(dialer *websocket.Dialer) *websocket.Dialer {
	if dialer == nil {
		return nil
	}
	cloned := *dialer
	cloned.Subprotocols = append([]string(nil), dialer.Subprotocols...)
	if dialer.TLSClientConfig != nil {
		cloned.TLSClientConfig = dialer.TLSClientConfig.Clone()
		cloned.TLSClientConfig.NextProtos = []string{"http/1.1"}
	}
	return &cloned
}

// WebSocketDialer returns a configurable snapshot, including private CA, mTLS,
// custom TCP dialing and proxy settings from a native HTTP transport.
func (client *Client) WebSocketDialer() *websocket.Dialer {
	client.websocketMu.RLock()
	dialer := cloneWebSocketDialer(client.websocketDialer)
	client.websocketMu.RUnlock()
	if dialer != nil {
		return dialer
	}
	dialer = cloneWebSocketDialer(websocket.DefaultDialer)
	if client.pool != nil {
		dialer.HandshakeTimeout = client.pool.Timeout
		dialer.Jar = client.pool.Jar
		if transport, ok := client.pool.Transport.(*http.Transport); ok {
			dialer.Proxy = transport.Proxy
			dialer.NetDialContext = transport.DialContext
			// Do not inherit DialTLSContext: a custom HTTP TLS dialer may negotiate h2.
			if transport.TLSClientConfig != nil {
				dialer.TLSClientConfig = transport.TLSClientConfig.Clone()
				dialer.TLSClientConfig.NextProtos = []string{"http/1.1"}
			}
		}
	}
	return dialer
}

// Clone shares the HTTP transport and cookie jar, while copying client settings
// and headers. It can bound one handshake without changing a shared client.
func (client *Client) Clone() *Client {
	cloned := &Client{NoBody: client.NoBody, Debug: client.Debug, DownloadPartSize: client.DownloadPartSize, MaxConnsPerHost: client.MaxConnsPerHost, globalHeaders: make(map[string]string)}
	if client.pool != nil {
		pool := *client.pool
		cloned.pool = &pool
	}
	client.headersMu.RLock()
	for key, value := range client.globalHeaders {
		cloned.globalHeaders[key] = value
	}
	client.headersMu.RUnlock()
	client.websocketMu.RLock()
	cloned.websocketDialer = cloneWebSocketDialer(client.websocketDialer)
	client.websocketMu.RUnlock()
	return cloned
}

// Gorilla retains at most 1024 bytes on bad handshakes. Publish the actual
// retained length, never the upstream's original framing or stale body length.
func normalizeWebSocketHandshake(response *http.Response) bool {
	if response.Body == nil {
		return false
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	truncated := response.ContentLength > int64(len(body)) || response.ContentLength < 0 && len(body) == 1024
	if truncated && response.Header.Get("Content-Encoding") != "" {
		body = []byte(http.StatusText(response.StatusCode))
		response.Header.Del("Content-Encoding")
		response.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.TransferEncoding = nil
	response.Header.Del("Transfer-Encoding")
	response.Header.Set("Content-Length", cast.String(len(body)))
	return truncated
}

// Send sends strings as text, byte slices as binary, and other values as JSON.
func (conn *WebSocketConn) Send(data any) error {
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	switch value := data.(type) {
	case string:
		return conn.Conn.WriteMessage(websocket.TextMessage, []byte(value))
	case []byte:
		return conn.Conn.WriteMessage(websocket.BinaryMessage, value)
	default:
		return conn.Conn.WriteJSON(value)
	}
}

func (conn *WebSocketConn) ReadString() (string, error) {
	conn.readMu.Lock()
	defer conn.readMu.Unlock()
	_, data, err := conn.Conn.ReadMessage()
	return string(data), err
}

func (conn *WebSocketConn) ReadBytes() ([]byte, error) {
	conn.readMu.Lock()
	defer conn.readMu.Unlock()
	_, data, err := conn.Conn.ReadMessage()
	return data, err
}

func (conn *WebSocketConn) ReadJSON(value any) error {
	conn.readMu.Lock()
	defer conn.readMu.Unlock()
	return conn.Conn.ReadJSON(value)
}

// SetReadDeadline sets the deadline for future reads. A zero value disables it.
func (conn *WebSocketConn) SetReadDeadline(deadline time.Time) error {
	return conn.Conn.SetReadDeadline(deadline)
}

// SetWriteDeadline sets the deadline for future writes. A zero value disables it.
func (conn *WebSocketConn) SetWriteDeadline(deadline time.Time) error {
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	return conn.Conn.SetWriteDeadline(deadline)
}

func (conn *WebSocketConn) Close() error {
	var err error
	conn.closeMu.Do(func() {
		close(conn.closed)
		err = conn.Conn.Close()
	})
	return err
}
