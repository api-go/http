package http

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketConn wraps a WebSocket connection with the common message helpers.
type WebSocketConn struct {
	Conn     *websocket.Conn
	Response *http.Response
}

// WebSocket connects with the default client.
func WebSocket(url string, headers ...string) (*WebSocketConn, error) {
	return DefaultClient.WebSocket(url, headers...)
}

// WebSocket connects to a WebSocket server using the client's timeout,
// cookies, and global headers.
func (client *Client) WebSocket(url string, headers ...string) (*WebSocketConn, error) {
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
	conn, response, err := dialer.Dial(url, requestHeaders)
	if err != nil {
		return nil, err
	}
	return &WebSocketConn{Conn: conn, Response: response}, nil
}

// Send sends strings as text, byte slices as binary, and other values as JSON.
func (conn *WebSocketConn) Send(data any) error {
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
	_, data, err := conn.Conn.ReadMessage()
	return string(data), err
}

func (conn *WebSocketConn) ReadBytes() ([]byte, error) {
	_, data, err := conn.Conn.ReadMessage()
	return data, err
}

func (conn *WebSocketConn) ReadJSON(value any) error {
	return conn.Conn.ReadJSON(value)
}

func (conn *WebSocketConn) Close() error {
	return conn.Conn.Close()
}
