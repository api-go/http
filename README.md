# github.com/api-go/http

`github.com/api-go/http` 是一个极致精简、高性能且安全的 HTTP 客户端与工具集. 它基于原生 `net/http` 构建，提供了更友好的 API、自动化的 Header 透传、并行分段下载支持以及泛型数据绑定。

## 核心特性

*   **极致精简**: 屏蔽复杂的 `net/http` 配置，提供一键式调用（Get/Post/Put/Delete/Head）。
*   **智能重构**: 基于 `cast` 和 `convert` 模块实现零摩擦的数据映射。
*   **并发下载**: 支持**并行多协程**分段下载大文件，内置自动重试机制。
*   **Header 透传**: 自动处理微服务链路中常见的 `X-` 系列 Header 透传（如 `X-Request-ID`, `X-Real-IP`）。
*   **线程安全**: 全局 Header 操作及客户端配置均实现并发安全，适合高并发场景。
*   **H2C 支持**: 原生支持 HTTP/2 Cleartext (h2c) 协议。
*   **WebSocket 客户端**: 复用客户端超时、Cookie 和全局 Header，支持 Context 取消、并发安全读写、Deadline、握手诊断以及文本、二进制和 JSON 消息。

## 🤖 开发与 AI 指导 (Developer & AI Guidelines)

1.  **推荐使用 NewClient**: 通过 `NewClient(timeout)` 创建带连接池的客户端。
3.  **下载优化**: 可以通过 `client.MaxConnsPerHost` 控制下载并发度（默认为 4）。
4.  **取消与资源释放**: 使用 `DoContext`/`ManualDoContext` 传播取消；使用 `ManualDo` 或直接访问 `Response.Body` 时，必须确保执行 `Close()`。

## 快速入门 (Quick Start)

### 1. 基础请求与泛型绑定
```go
import "github.com/api-go/http"

c := http.NewClient(time.Second * 5)

// 发起请求并解析 JSON
type User struct {
    ID   int
    Name string
}

r := c.Get("https://api.example.com/user/1")
user, err := http.To[User](r)
if err == nil {
    fmt.Println(user.Name)
}

// 支持 map[string][]string 或 map[string][]any 作为表单 (application/x-www-form-urlencoded)
c.Post("https://api.example.com/update", map[string][]string{"tags": {"go", "http"}})
```


### 2. Header 透传 (Discover Relay)
在处理外部请求时，自动从原请求中提取并续传关键 Header。
```go
func MyHandler(w http.ResponseWriter, r *http.Request) {
    c := http.NewClient(time.Second)
    // 自动透传 X-Request-ID, X-Real-IP 等
    res := c.DoByRequest(r, "GET", "http://internal-service/api", nil)
    fmt.Println(res.String())
}
```

### 3. 并发下载
```go
c := http.NewClient(0)
c.MaxConnsPerHost = 8 // 设置并行连接数
c.Download("local_file.zip", "https://example.com/large_file.zip", func(start, end int64, ok bool, finished, total int64) {
    fmt.Printf("Progress: %0.2f%%\n", float64(finished)/float64(total)*100)
})
```

### 4. WebSocket
```go
conn, response, err := c.Open("wss://example.com/ws", http.Header{"Authorization": {"Bearer token"}})
_ = response // 成功或失败均返回握手响应
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

_ = conn.Send(map[string]any{"action": "ping"})
var reply map[string]any
_ = conn.ReadJSON(&reply)
```

## 🛠 API Reference

### 客户端创建
- `func NewClient(timeout time.Duration) *Client`: 创建标准 HTTP 客户端。
- `func NewClientH2C(timeout time.Duration) *Client`: 创建支持 H2C 的 HTTP 客户端。
- `func Open(url string, headers ...http.Header) (*WebSocketConn, *http.Response, error)`: 使用默认客户端建立 WebSocket 连接。
- `func OpenContext(ctx context.Context, url string, headers ...http.Header) (*WebSocketConn, *http.Response, error)`: 建立随 Context 取消而关闭的 WebSocket 连接。
- `func (c *Client) Open(url string, headers ...http.Header) (*WebSocketConn, *http.Response, error)`: 建立复用客户端配置的 WebSocket 连接。
- `func (c *Client) OpenContext(ctx context.Context, url string, headers ...http.Header) (*WebSocketConn, *http.Response, error)`: 建立随 Context 取消而关闭的 WebSocket 连接。

### 请求方法
- `func (c *Client) Get(url string, headers ...string) *Result`
- `func (c *Client) Post(url string, data any, headers ...string) *Result`: 支持多种数据类型（JSON, Form, Multipart）。
- `func (c *Client) Put(url string, data any, headers ...string) *Result`
- `func (c *Client) Delete(url string, data any, headers ...string) *Result`
- `func (c *Client) Head(url string, headers ...string) *Result`

### 特殊类型
- `type Form map[string]string`: 用于 `Post/Put` 等方法，显式指定为 `application/x-www-form-urlencoded` 格式。
    - 注意：直接传入 `map[string]string` 会被默认识别为 `application/json`；传入 `map[string][]string` 或 `map[string][]any` 会被识别为 `application/x-www-form-urlencoded`。
- `type Multipart map[string]any`: 用于 `Post/Put` 等方法，支持混合表单字段与文件流式上传。
    - **流式上传**: 内部使用 `io.Pipe` 结合 `multipart.Writer` 实现，支持超大文件而不会占用过多内存。
    - **文件识别**: 如果值为 `string` 且指向有效文件路径，则作为文件上传。
    - **流/字节**: 如果值为 `[]byte` 或 `io.Reader`，则作为文件上传。
    - **多值支持**: 如果值为 `[]string` 或 `[]any`，将产生多个同名的表单字段或文件。
    - **其他类型**: 将作为普通表单字段（复杂类型会自动转为 JSON）。

### 响应处理 (Result)
- `func (rs *Result) String() string`: 返回响应体字符串。
- `func (rs *Result) Bytes() []byte`: 返回响应体字节数组。
- `func (rs *Result) To(v any) error`: 将响应体解析到对象。
- `func (rs *Result) Map() map[string]any`: 将响应体解析为 Map。
- `func (rs *Result) Slice() []any`: 将响应体解析为 Slice。
- `func To[T any](rs *Result) (T, error)`: 泛型解析辅助函数。
- `func (rs *Result) Save(filename string) error`: 将响应体保存到文件。

### WebSocket 连接
- `func (c *WebSocketConn) Send(data any) error`: 字符串发送为文本、字节切片发送为二进制，其他值编码为 JSON。
- `func (c *WebSocketConn) ReadString() (string, error)`
- `func (c *WebSocketConn) ReadBytes() ([]byte, error)`
- `func (c *WebSocketConn) ReadJSON(v any) error`
- `func (c *WebSocketConn) SetReadDeadline(deadline time.Time) error`
- `func (c *WebSocketConn) SetWriteDeadline(deadline time.Time) error`
- `func (c *WebSocketConn) Close() error`

同一连接允许一个读取者和一个写入者并行使用；同方向的并发操作会被串行化。升级失败时错误可通过 `errors.As` 转换为 `*WebSocketHandshakeError` 并读取 HTTP 响应，读取后应关闭 `Response.Body`。

## 许可证
本项目基于 MIT 许可证开源。

## v1.5.11 依赖基线

本版本对齐 Log v1.5.17 与发布时模块索引中的上游已发布依赖；公开 API 保持兼容。

## WebSocket Open 接口

客户端 Go 与 JS 的 WebSocket 建连接口统一为 Open（Go 另有 OpenContext），删除旧 WebSocket / WebSocketContext 名称。Go 使用 http.Header，保留多值字段，并返回连接、握手响应、错误。失败正文最多保留 1024 字节，Content-Length 已归一化，WebSocketHandshakeError.BodyTruncated 标记截断。

- `Client.SetWebSocketDialer(*websocket.Dialer)`：设置独立配置快照，nil 恢复 HTTP Transport 派生默认；支持私有 CA、mTLS、自定义拨号和压缩。TLS ALPN 固定 HTTP/1.1；自定义 NetDialTLSContext 必须自行遵守此协议。
- `Client.WebSocketDialer() *websocket.Dialer`：取得配置副本。
- `Client.Clone() *Client`：复制客户端配置和 Header，共享 Transport / CookieJar；配置应在服务开始前设置，Clone 不隔离共享连接池。

## JavaScript 请求契约（v1.5.13）

`http.New(timeoutMs)` 的超时单位为毫秒。GET/POST/PUT/DELETE 返回可读取 `Status()`、`String()`、`Map()`、`Error()` 的结果；传输失败保留在 `Error()`，调用方决定如何记录。请求继承当前调用 Context 的取消。

```javascript
const result = http.New(5000).GET(url, {})
const healthy = !result.Error() && result.Status() === 200 && result.Map().ok === true
```

## HTTP/2 WebSocket（v1.5.14）

Open/OpenContext 默认自动选择协议：`wss://` 优先 H2，只有 ALPN 未协商 H2 或服务端未声明 RFC 8441 时退回 H1；普通 `ws://` 直接 H1，不探测 H2C。显式使用 NewClientH2C 的客户端仍可优先 H2C。不会从 WSS 降级到明文 WS，也不会因证书错误、超时、401/403、其他 HTTP 拒绝而重试。协议不支持的结果按目标地址缓存一分钟，最多 128 项；Clone 共享缓存与 H2 连接池。

无需应用配置。`Client.SetWebSocketHTTP2(true)` 保留严格 H2、不降级；false 固定 H1，供需要明确协议的调用使用。自定义代理/TLS 拨号及独立 Gorilla 网络配置优先保持其网络策略，自动模式可继续走 H1；显式 http2.Transport 自行管理网络策略。TLS CA/mTLS 配置应在客户端使用前安装在 HTTP Transport 上。

`IsWebSocketRequest` 识别两种握手；`UpgradeWebSocket` 复用 Gorilla Upgrader，处理函数须保持到连接关闭。Go/x/net 的服务端能力声明仍需在进程启动前设置 `GODEBUG=http2xconnect=1`，运行后设置不会生效；未设置时自动客户端保持兼容 H1。成功 H2 握手返回真实 200，关闭其 Body 不关闭隧道。Destroy 关闭空闲连接池。

`Client.SetWebSocketOptions(compression bool, writeBufferPool websocket.BufferPool)` 设置压缩与缓冲池，同时保留自动协议协商和 HTTP Transport 的网络策略；独立 SetWebSocketDialer 的网络设置仍优先。

## 完整客户端控制接口

- 包级 `Get/Post/Put/Delete/Do` 使用 `DefaultClient`；`Do(method, url, data, headers...)` 支持显式方法。
- `Client.GetRawClient() *http.Client` 暴露底层客户端；`EnableRedirect()` 启用跟随重定向；`SetGlobalHeader/GetGlobalHeader` 管理客户端共享 Header。
- `Client.Do/ManualDo(method, url, data, headers...)` 与 `DoContext/ManualDoContext(ctx, ...)` 分别提供普通/手动响应处理和取消传播。
- `Client.DoByRequest/ManualDoByRequest(request, method, url, data, headers...)` 从入口请求继承调用上下文与转发 Header。
- `Client.Destroy()` 关闭空闲连接池并释放客户端；调用后不再使用该实例。
- `Result` 保留 `Response *http.Response` 与 `Error error`；`DefaultClient` 是包级默认客户端。`WebSocketConn` 保留 Gorilla `Conn` 与实际握手 `Response`。

## External address / 外部地址（v1.5.16）

`NewExternalAddressResolver(ExternalAddressPolicy{TrustedProxies: []string{"192.0.2.10/32"}})` validates an immutable policy. Call `resolver.Prepare(request)` at ingress before sharing or mutating the request. Default behavior (v1.5.16): an empty policy uses the existing internal Request-ID relay convention. Without an incoming Request-ID it uses actual TLS/Host; with an incoming ID it preserves supported address pairs. Preparation must happen before generating a missing ID.

`RequestExternalAddress(request)` returns the pinned `ExternalAddress` (`Scheme`, canonical `Host`, `Hostname`, effective `Port`, `Origin`, `TrustedProxy`, `Err`). Without preparation it pins the default relay-policy result. `IsHTTPS`, `SameOrigin(headers)`, `RelayHeaders(headers)` share the result. Check Err; reject invalid trusted metadata before business/security processing. Never treat this context as RPC destination or authentication.

X-Host/X-Scheme and X-Forwarded-Host/Proto must each be complete single-value pairs; either pair is accepted, both must normalize to the same origin. Duplicates, comma lists and conflicts fail. Forwarded is not parsed: alone from a trusted peer it fails; beside supported pairs it is ignored and cleared on relay. Every proxy hop must overwrite single-value pairs. Untrusted forwarding headers are ignored and replaced on relay. Origin remains untouched; no automatic Origin propagation is added to RPC clients.

SameOrigin compares scheme, case-insensitive hostname and effective port, including IPv6. Missing Origin remains allowed for compatibility; empty, null, repeated, list, credential, path, query and fragment origins fail. No global browser-origin enforcement is installed.

`PreparedExternalAddress(request) (ExternalAddress, bool)` distinguishes ingress preparation (including errors) from legacy raw context; its boolean is not a proxy-trust credential. `RequestExternalAddress` pins the default relay-policy snapshot when no ingress resolver has run.

## 默认可用 / Default relay (v1.5.16)

No proxy configuration is needed for controlled internal service chains: the outermost gateway writes canonical addressing; Service resolves before generating a missing Request-ID, and internal hops with an incoming ID preserve that address. Origin remains unchanged. Request-ID is the existing relay convention, not RPC authentication or a CIDR trust assertion; TrustedProxy is true only for a matched explicitly configured proxy.

Optional `ExternalAddressPolicy.Mode`: empty/legacy uses this convention; direct always rebuilds from actual TLS/Host even with an incoming ID; trusted accepts address pairs only from configured TrustedProxies and requires at least one address/CIDR. Supplying TrustedProxies without Mode selects trusted mode, preserving the v1.5.15 explicit-policy behavior. A public Gateway business ingress defaults to direct so external headers are overwritten even if the client sends an ID. Management and internal Service default to legacy. Explicit forward-proxy policies remain available as enhancements.
