package http

import (
	"context"
	"net/http"
	"sync"
	"time"

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
	Response *http.Response
	Err      error
}

func (err *WebSocketHandshakeError) Error() string { return err.Err.Error() }
func (err *WebSocketHandshakeError) Unwrap() error { return err.Err }

// WebSocket connects with the default client.
func WebSocket(url string, headers ...string) (*WebSocketConn, error) {
	return DefaultClient.WebSocketContext(context.Background(), url, headers...)
}

// WebSocketContext connects with the default client and closes the connection
// when ctx is cancelled.
func WebSocketContext(ctx context.Context, url string, headers ...string) (*WebSocketConn, error) {
	return DefaultClient.WebSocketContext(ctx, url, headers...)
}

// WebSocket connects to a WebSocket server using the client's timeout,
// cookies, and global headers.
func (client *Client) WebSocket(url string, headers ...string) (*WebSocketConn, error) {
	return client.WebSocketContext(context.Background(), url, headers...)
}

// WebSocketContext connects to a WebSocket server using the client's timeout,
// cookies, and global headers. Cancelling ctx closes an established connection.
func (client *Client) WebSocketContext(ctx context.Context, url string, headers ...string) (*WebSocketConn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestHeaders := make(http.Header)
	for i := 1; i < len(headers); i += 2 {
		requestHeaders.Set(headers[i-1], headers[i])
	}

	client.headersMu.RLock()
	for key, value := range client.globalHeaders {
		requestHeaders.Set(key, value)
	}
	client.headersMu.RUnlock()

	timeout := time.Duration(0)
	var jar http.CookieJar
	if client.pool != nil {
		timeout = client.pool.Timeout
		jar = client.pool.Jar
	}

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = timeout
	dialer.Jar = jar
	conn, response, err := dialer.DialContext(ctx, url, requestHeaders)
	if err != nil {
		if response != nil {
			return nil, &WebSocketHandshakeError{Response: response, Err: err}
		}
		return nil, err
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
	return wrapped, nil
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
