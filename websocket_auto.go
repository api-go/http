package http

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

var errWebSocketHTTP2Unavailable = errors.New("websocket: peer did not negotiate HTTP/2")

// Keep unsupported origins bounded and retry capability negotiation after one
// minute, so backend upgrades do not require restarting clients.
type webSocketFallbackCache struct {
	mu      sync.Mutex
	origins map[string]time.Time
}

func (cache *webSocketFallbackCache) unsupported(origin string) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	deadline, ok := cache.origins[origin]
	if ok && time.Now().Before(deadline) {
		return true
	}
	delete(cache.origins, origin)
	return false
}
func (cache *webSocketFallbackCache) remember(origin string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.origins) >= 128 {
		for key := range cache.origins {
			delete(cache.origins, key)
			break
		}
	}
	cache.origins[origin] = time.Now().Add(time.Minute)
}

func (client *Client) openWebSocketAuto(ctx context.Context, target string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, nil, err
	}
	eligible := parsed.Scheme == "wss"
	if transport, ok := client.pool.Transport.(*http2.Transport); ok && transport.AllowHTTP && parsed.Scheme == "ws" {
		eligible = true
	}
	// Preserve independent network/proxy settings. A custom HTTP/2 transport
	// explicitly owns its dialing policy; ordinary ws:// never probes H2C.
	switch transport := client.pool.Transport.(type) {
	case nil, *http2.Transport:
	case *http.Transport:
		if transport.DialTLSContext != nil || transport.DialTLS != nil {
			eligible = false
		}
	default:
		eligible = false
	}
	client.websocketMu.RLock()
	custom := client.websocketDialer
	if custom != nil && !client.websocketDialerDerived {
		if custom.NetDial != nil || custom.NetDialContext != nil || custom.NetDialTLSContext != nil || custom.TLSClientConfig != nil {
			eligible = false
		}
	}

	client.websocketMu.RUnlock()
	dialer := client.WebSocketDialer()
	if eligible && dialer.Proxy != nil {
		probe := &http.Request{URL: parsed, Header: make(http.Header)}
		probe.URL = cloneWebSocketHTTPURL(parsed)
		proxy, proxyErr := dialer.Proxy(probe)
		if proxyErr != nil {
			return nil, nil, proxyErr
		}
		if proxy != nil {
			eligible = false
		}
	}
	if !eligible {
		return client.openWebSocketHTTP1(ctx, target, headers...)
	}
	client.prepareWebSocketHTTP2()
	client.websocketMu.RLock()
	cache := client.websocketHTTP2Fallback
	client.websocketMu.RUnlock()
	origin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	if cache.unsupported(origin) {
		return client.openWebSocketHTTP1(ctx, target, headers...)
	}
	// Bound the entire operation, including a safe fallback, to one handshake
	// budget. Cancel the derived context on Close, not on successful Open.
	handshakeCtx, cancelCause := context.WithCancelCause(ctx)
	cancel := func() { cancelCause(context.Canceled) }
	timeout := dialer.HandshakeTimeout
	if client.pool.Timeout > 0 && (timeout <= 0 || client.pool.Timeout < timeout) {
		timeout = client.pool.Timeout
	}
	var timer *time.Timer
	if timeout > 0 {
		timer = time.AfterFunc(timeout, func() { cancelCause(context.DeadlineExceeded) })
	}
	conn, response, err := client.openWebSocketHTTP2(handshakeCtx, target, headers...)
	// x/net v0.54.0 keeps its SETTINGS refusal sentinel private. This exact
	// transport error occurs before CONNECT is sent; never retry HTTP responses.
	unavailable := errors.Is(err, errWebSocketHTTP2Unavailable) || err != nil && err.Error() == "net/http: extended connect not supported by peer"
	if err != nil && response == nil && unavailable && handshakeCtx.Err() == nil {
		cache.remember(origin)
		conn, response, err = client.openWebSocketHTTP1(handshakeCtx, target, headers...)
	}
	if timer != nil {
		timer.Stop()
	}
	if err != nil {
		if cause := context.Cause(handshakeCtx); cause != nil {
			err = cause
		}
		cancel()
		return nil, response, err
	}
	if handshakeCtx.Err() != nil {
		conn.Close()
		cancel()
		return nil, response, handshakeCtx.Err()
	}
	conn.cancelMu.Lock()
	select {
	case <-conn.closed:
		cancel()
	default:
		conn.cancel = cancel
	}
	conn.cancelMu.Unlock()
	return conn, response, nil
}
func cloneWebSocketHTTPURL(source *url.URL) *url.URL {
	cloned := *source
	if cloned.Scheme == "wss" {
		cloned.Scheme = "https"
	} else if cloned.Scheme == "ws" {
		cloned.Scheme = "http"
	}
	return &cloned
}
func (client *Client) openWebSocketHTTP1(ctx context.Context, target string, headers ...http.Header) (*WebSocketConn, *http.Response, error) {
	cloned := client.Clone()
	cloned.SetWebSocketHTTP2(false)
	return cloned.OpenContext(ctx, target, headers...)
}
