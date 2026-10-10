# apigo.cc/go/http

`apigo.cc/go/http` 是一个极致精简、高性能且安全的 HTTP 客户端与工具集. 它基于原生 `net/http` 构建，提供了更友好的 API、自动化的 Header 透传、并行分段下载支持以及泛型数据绑定。

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
import "apigo.cc/go/http"

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
conn, err := c.WebSocket("wss://example.com/ws", "Authorization", "Bearer token")
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
- `func WebSocket(url string, headers ...string) (*WebSocketConn, error)`: 使用默认客户端建立 WebSocket 连接。
- `func WebSocketContext(ctx context.Context, url string, headers ...string) (*WebSocketConn, error)`: 建立随 Context 取消而关闭的 WebSocket 连接。
- `func (c *Client) WebSocket(url string, headers ...string) (*WebSocketConn, error)`: 建立复用客户端配置的 WebSocket 连接。
- `func (c *Client) WebSocketContext(ctx context.Context, url string, headers ...string) (*WebSocketConn, error)`: 建立随 Context 取消而关闭的 WebSocket 连接。

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
