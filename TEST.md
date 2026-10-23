# 测试报告

## v1.5.13 发布验证（2026-10-21）
- `TestJavaScriptHTTPResultAndCancellation` 覆盖静态/客户端 GET、DELETE 的结果返回形状、200/503 正文和调用 Context 超时。
- Master 最小 JS 入口验证 HTTP 200、JSON正文、Error() 和生成声明一致；测试覆盖静态/客户端安全模式 DELETE 拒绝发送未授权本地文件。
- GOWORK=off 全量、race、基准通过；BenchmarkGet 34743 ns/op。API 使用 go doc 与导出源码交叉核对。

## v1.5.12 验证
- `TestOpenCancelsPendingHandshake` 验证取消不等待握手超时；`TestOpenRejectsUnofferedSubprotocol` 验证子协议边界。
- 2026-10-14：GOWORK=off 全量、基准及 Open/WebSocket 专项 race（2 次）通过；Apple M3 Max，BenchmarkGet 31617 ns/op。API 使用 go doc -all 与源码核对。
- Open 覆盖多值头、请求/响应 Cookie、HTTP2 Transport 私有 CA 继承、HTTP1 WebSocket 协商、配置不被修改、超时、取消及长拒绝正文长度归一化。
- 公共 API 使用 GOWORK=off go doc 与导出源码核对。

## v1.5.10 验证
- `TestWebSocketContextCancellation` 验证 Context 取消会关闭连接并中断阻塞读取。
- `TestWebSocketHandshakeErrorRetainsResponse` 验证升级失败保留 HTTP 状态与响应 Header。
- WebSocket 读写锁与幂等关闭覆盖托管连接复用所需的并发边界。

## v1.5.9 验证
- `TestDoContextCancelsInFlightRequest` 验证 Context 取消会及时终止在途请求。
- `go test -v ./...` 与 `go test -bench=. ./...` 通过。

## v1.5.7 验证
- 独立模块解析、全量测试与基准测试均通过。

## 测试场景覆盖
- **基础请求**: 验证了使用 `Get` 获取 JSON 响应并通过 `To[T]` 进行泛型绑定的功能。
- **本地服务器**: 验证了客户端与本地测试服务器的交互。
- **H2C 支持**: 验证了 H2C (HTTP/2 Cleartext) 的兼容性。
- **手动请求**: 验证了通过 `ManualDo` 进行精细化控制的流式请求。
- **文件下载**: 验证了使用 `Download` 进行并发分段下载的功能。
- **多部分表单 (Multipart)**: 验证了使用流式 `io.Pipe` 的 `Multipart` 发送，以及多类型、多值字段的支持。
- **表单与映射**: 验证了使用 `Form`, `map[string]string`, `map[string][]string`, 和 `map[string][]any` 自动转为正确的 `application/x-www-form-urlencoded` 或 `application/json`。
- **WebSocket 客户端**: 验证了握手响应、全局与单次 Header、Cookie Jar，以及文本、二进制和 JSON 消息往返。

## 性能测试结果 (Benchmark)
```
goos: darwin
goarch: arm64
pkg: apigo.cc/go/http
cpu: Apple M3 Max
BenchmarkGet-16    	   30687	     36867 ns/op
```

## 🛡️ 鲁棒性防御 (Robustness)
- **可选参数与验证**：`POST`/`PUT` 请求自动前置验证 `Multipart` 文件的安全路径，并受安全沙箱控制。
- **JS 错误调用栈**：JS 桥接层改用具名导出并使用 `jsmod.MakeError` 包裹错误，确保 JS 抛出异常时携带准确的 Go 运行时堆栈。

## 测试覆盖率
- **整体覆盖率**: 49.6% of statements

## v1.5.11 发布验证（2026-10-10）

- GOWORK=off go test -v ./... 与 go test -bench=. ./... 通过，依赖均为已发布 tag。
- API 以 GOWORK=off go doc 与导出源码交叉核对（getExports 工具不可用）。

```text
BenchmarkGet-16    	   18500	     66281 ns/op
```

## v1.5.14 HTTP/2 WebSocket 发布验证（2026-10-23）

验证命令：`GOWORK=off GODEBUG=http2xconnect=1 go test -race -v ./...` 与 `GOWORK=off GODEBUG=http2xconnect=1 go test -bench=. ./...`。

覆盖：自动 WSS→H2；ALPN 不支持 H2 时 H1 回退；独立子进程服务端不声明 RFC 8441 时 H1 回退；不重复发送 CONNECT；跨 Clone 的一分钟有界能力缓存；明文 WS 直接 H1；显式 H2C；私有 CA 继承、证书拒绝、独立 TLS Dialer 保留；401 不降级；子协议、多值 Cookie、消息、取消、Deadline、单流关闭、多流复用。显式 SetWebSocketHTTP2(false) 验证旧 H1 行为，true 验证严格 H2。API 使用 go doc -all 与导出源码交叉核对。

TLS 对照实测、完整方法、三轮原始输出见 [性能报告](testdata/websocket/20261022-tls-compare.md)。Apple M3 Max / Go 1.26.4：1 KiB 单流回显中位数 H1 19.304 µs、H2 44.054 µs；16 流有效载荷吞吐 H1 98.67 MB/s、H2 79.69 MB/s，TCP 连接分别 16 与 1；warm 新建 WebSocket H1 1.574 ms、H2 0.107 ms。

2026-10-23 独立发布检查：关闭工作区（`GOWORK=off`），默认启动及 `GODEBUG=http2xconnect=1` 两种模式的全量 `go test -v ./... -count=1`、`go vet ./...`、`go test -race ./... -count=1`、`go test -run=^$ -bench=. -benchmem ./...` 均通过；默认模式只跳过需服务端启动声明的严格 H2 专项，其 H1 回退仍实测。`ai/tools/getExports` 不可用，使用 `GOWORK=off go doc -all .` 与导出源码交叉核对；新导出为 `IsWebSocketRequest`、`UpgradeWebSocket`、`Client.SetWebSocketHTTP2` 和 `Client.SetWebSocketOptions`。

Apple M3 Max 本轮单次基准（非三轮对照中位数）：BenchmarkGet 默认 34887 ns/op，H2 启动模式 34962 ns/op；H2Echo 35688 ns/op；TLS 1 KiB 单流 H1/H2 17956/41477 ns/op，warm 建连 H1/H2 1459146/103172 ns/op。历史三轮报告保留原日期和范围。

## External address validation (2026-10-23)
- Direct TLS/HTTP, explicit proxy trust, spoofed headers, paired header conflicts/lists/duplicates, IPv6, ports, pinned snapshots and strict Origin structure.
- Commands: GOWORK=off go test -v ./...; GOWORK=off go test -bench=. ./... .
- API review fallback: GOWORK=off go doc plus exported source review (getExports unavailable).

- Independent GOWORK=off full suite and race suite passed. Apple M3 Max: BenchmarkExternalAddress 1610 ns/op (includes httptest request creation); BenchmarkGet 35592 ns/op.

## v1.5.16 default relay validation (2026-10-23)
- First hop without incoming ID uses actual connection; incoming ID preserves outer HTTPS, even without any proxy configuration. Later ID generation cannot change a pinned result. Optional direct and trusted modes override the default convention. Invalid mode and trusted-without-addresses are rejected.

- Development validation passed: GOWORK=off go test -v ./..., go test -bench=. ./..., go test -race ./... and go vet ./.... Apple M3 Max: Default resolver 1681 ns/op; HTTP GET 36489 ns/op. HTTP is tested independently; downstream modules use documented explicit local replace mappings.

- v1.5.16 发布复验：`GOWORK=off go test -v ./...` 与 `go test -bench=. ./...` 全部通过，无 replace；API 通过 `go doc -all .` 与导出源码核对（getExports 不可用）。
