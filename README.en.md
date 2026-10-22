# apigo.cc/go/http v1.5.14

[中文 API 文档](README.md)

Open/OpenContext automatically prefer RFC 8441 for WSS. Fallback to HTTP/1.1 only when ALPN or peer SETTINGS do not support it, before any CONNECT is sent. Certificate failures, timeouts and HTTP rejections never trigger fallback. Ordinary WS uses H1; explicit NewClientH2C clients may use H2C. Unsupported origins are cached for one minute, bounded to 128 entries; clones share the cache and H2 pool. Independent proxy/dialer policies retain H1 when they cannot be represented by the H2 transport. WSS never falls back to plaintext.

SetWebSocketHTTP2(true) explicitly requires H2; false forces H1. The default needs no application switch. Configure TLS trust/mTLS through the HTTP transport before use.

IsWebSocketRequest recognizes H1 and H2. UpgradeWebSocket reuses Gorilla framing and upgrader policies; keep the handler active for the connection lifetime. Go/x/net still requires server startup environment GODEBUG=http2xconnect=1 to advertise RFC 8441. Changing it after initialization is ineffective. Successful H2 handshakes expose HTTP/2 200; closing the response body preserves the live tunnel. Destroy closes idle pools.

Performance measurements, raw results and methodology: [TLS comparison](testdata/websocket/20261022-tls-compare.md).

`Client.SetWebSocketOptions(compression bool, writeBufferPool websocket.BufferPool)` configures framing options while preserving automatic protocol negotiation and the HTTP transport network policy. Independent SetWebSocketDialer network settings remain authoritative.

The complete client, response and JavaScript API inventory is maintained in the linked Chinese API reference. Existing HTTP request and HTTP/1.1 WebSocket APIs remain available.
