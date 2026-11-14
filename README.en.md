# github.com/api-go/http v1.5.14

[中文 API 文档](README.md)

Open/OpenContext automatically prefer RFC 8441 for WSS. Fallback to HTTP/1.1 only when ALPN or peer SETTINGS do not support it, before any CONNECT is sent. Certificate failures, timeouts and HTTP rejections never trigger fallback. Ordinary WS uses H1; explicit NewClientH2C clients may use H2C. Unsupported origins are cached for one minute, bounded to 128 entries; clones share the cache and H2 pool. Independent proxy/dialer policies retain H1 when they cannot be represented by the H2 transport. WSS never falls back to plaintext.

SetWebSocketHTTP2(true) explicitly requires H2; false forces H1. The default needs no application switch. Configure TLS trust/mTLS through the HTTP transport before use.

IsWebSocketRequest recognizes H1 and H2. UpgradeWebSocket reuses Gorilla framing and upgrader policies; keep the handler active for the connection lifetime. Go/x/net still requires server startup environment GODEBUG=http2xconnect=1 to advertise RFC 8441. Changing it after initialization is ineffective. Successful H2 handshakes expose HTTP/2 200; closing the response body preserves the live tunnel. Destroy closes idle pools.

Performance measurements, raw results and methodology: [TLS comparison](testdata/websocket/20261022-tls-compare.md).

`Client.SetWebSocketOptions(compression bool, writeBufferPool websocket.BufferPool)` configures framing options while preserving automatic protocol negotiation and the HTTP transport network policy. Independent SetWebSocketDialer network settings remain authoritative.

The complete client, response and JavaScript API inventory is maintained in the linked Chinese API reference. Existing HTTP request and HTTP/1.1 WebSocket APIs remain available.

## External address / 外部地址（v1.5.16）

`NewExternalAddressResolver(ExternalAddressPolicy{TrustedProxies: []string{"192.0.2.10/32"}})` validates an immutable policy. Call `resolver.Prepare(request)` at ingress before sharing or mutating the request. Default behavior (v1.5.16): an empty policy uses the existing internal Request-ID relay convention. Without an incoming Request-ID it uses actual TLS/Host; with an incoming ID it preserves supported address pairs. Preparation must happen before generating a missing ID.

`RequestExternalAddress(request)` returns the pinned `ExternalAddress` (`Scheme`, canonical `Host`, `Hostname`, effective `Port`, `Origin`, `TrustedProxy`, `Err`). Without preparation it pins the default relay-policy result. `IsHTTPS`, `SameOrigin(headers)`, `RelayHeaders(headers)` share the result. Check Err; reject invalid trusted metadata before business/security processing. Never treat this context as RPC destination or authentication.

X-Host/X-Scheme and X-Forwarded-Host/Proto must each be complete single-value pairs; either pair is accepted, both must normalize to the same origin. Duplicates, comma lists and conflicts fail. Forwarded is not parsed: alone from a trusted peer it fails; beside supported pairs it is ignored and cleared on relay. Every proxy hop must overwrite single-value pairs. Untrusted forwarding headers are ignored and replaced on relay. Origin remains untouched; no automatic Origin propagation is added to RPC clients.

SameOrigin compares scheme, case-insensitive hostname and effective port, including IPv6. Missing Origin remains allowed for compatibility; empty, null, repeated, list, credential, path, query and fragment origins fail. No global browser-origin enforcement is installed.

`PreparedExternalAddress(request) (ExternalAddress, bool)` distinguishes ingress preparation (including errors) from legacy raw context; its boolean is not a proxy-trust credential. `RequestExternalAddress` pins the default relay-policy snapshot when no ingress resolver has run.

## 默认可用 / Default relay (v1.5.16)

No proxy configuration is needed for controlled internal service chains: the outermost gateway writes canonical addressing; Service resolves before generating a missing Request-ID, and internal hops with an incoming ID preserve that address. Origin remains unchanged. Request-ID is the existing relay convention, not RPC authentication or a CIDR trust assertion; TrustedProxy is true only for a matched explicitly configured proxy.

Optional `ExternalAddressPolicy.Mode`: empty/legacy uses this convention; direct always rebuilds from actual TLS/Host even with an incoming ID; trusted accepts address pairs only from configured TrustedProxies and requires at least one address/CIDR. Supplying TrustedProxies without Mode selects trusted mode, preserving the v1.5.15 explicit-policy behavior. A public Gateway business ingress defaults to direct so external headers are overwritten even if the client sends an ID. Management and internal Service default to legacy. Explicit forward-proxy policies remain available as enhancements.

### Realtime log integration (unreleased, 2026-11-12)

Released in v1.5.18 with published dependencies and no development replacements. File logs use paired .gz/.dict files; read through logv or embedded commands. Keep both files together; NoCompress retains plaintext.


### Release v1.5.18 — 2026-11-12

Released GitHub dependencies; no local development replacements. Full tests and benchmarks validated with GOWORK=off.

v1.5.19: dependency alignment with safe v1.5.6 / crypto v1.5.11. Development uses explicit local replacements; remove before release. Public API unchanged.

Release v1.5.19: verified with published dependencies and GOWORK=off; all temporary replacements have been removed. Earlier development-validation notes describe historical local checks.
