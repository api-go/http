# 测试报告

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
