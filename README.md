# apigo.cc/go/http

`apigo.cc/go/http` 是一个极致精简、高性能且安全的 HTTP 客户端与工具集. 它基于原生 `net/http` 构建，提供了更友好的 API、自动化的 Header 透传、并行分段下载支持以及泛型数据绑定。

## 核心特性

*   **极致精简**: 屏蔽复杂的 `net/http` 配置，提供一键式调用（Get/Post/Put/Delete/Head）。
*   **智能重构**: 基于 `cast` 和 `convert` 模块实现零摩擦的数据映射。
*   **并发下载**: 支持**并行多协程**分段下载大文件，内置自动重试机制。
*   **Header 透传**: 自动处理微服务链路中常见的 `X-` 系列 Header 透传（如 `X-Request-ID`, `X-Real-IP`）。
*   **线程安全**: 全局 Header 操作及客户端配置均实现并发安全，适合高并发场景。
*   **H2C 支持**: 原生支持 HTTP/2 Cleartext (h2c) 协议。

## 🤖 开发与 AI 指导 (Developer & AI Guidelines)

1.  **推荐使用 NewClient**: 通过 `NewClient(timeout)` 创建带连接池的客户端。
3.  **下载优化**: 可以通过 `client.MaxConnsPerHost` 控制下载并发度（默认为 4）。
4.  **资源释放**: 使用 `ManualDo` 或直接访问 `Response.Body` 时，必须确保执行 `Close()`。

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

## 🛠 API Reference

### 客户端创建
- `func NewClient(timeout time.Duration) *Client`: 创建标准 HTTP 客户端。
- `func NewClientH2C(timeout time.Duration) *Client`: 创建支持 H2C 的 HTTP 客户端。

### 请求方法
- `func (c *Client) Get(url string, headers ...string) *Result`
- `func (c *Client) Post(url string, data any, headers ...string) *Result`
- `func (c *Client) Put(url string, data any, headers ...string) *Result`
- `func (c *Client) Delete(url string, data any, headers ...string) *Result`
- `func (c *Client) Head(url string, headers ...string) *Result`
- `func (c *Client) PostMultipart(url string, formData map[string]string, files map[string]any, headers ...string) (*Result, []error)`: 多部分表单提交（支持文件与流）。

### 响应处理 (Result)
- `func (rs *Result) String() string`: 返回响应体字符串。
- `func (rs *Result) Bytes() []byte`: 返回响应体字节数组。
- `func (rs *Result) To(v any) error`: 将响应体解析到对象。
- `func (rs *Result) Map() map[string]any`: 将响应体解析为 Map。
- `func (rs *Result) Slice() []any`: 将响应体解析为 Slice。
- `func To[T any](rs *Result) (T, error)`: 泛型解析辅助函数。
- `func (rs *Result) Save(filename string) error`: 将响应体保存到文件。

## 许可证
本项目基于 MIT 许可证开源。
