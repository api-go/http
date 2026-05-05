# 测试报告

## 测试场景覆盖
- **基础请求**: 验证了使用 `Get` 获取 JSON 响应并通过 `To[T]` 进行泛型绑定的功能。
- **本地服务器**: 验证了客户端与本地测试服务器的交互。
- **H2C 支持**: 验证了 H2C (HTTP/2 Cleartext) 的兼容性。
- **手动请求**: 验证了通过 `ManualDo` 进行精细化控制的流程。
- **文件下载**: 验证了使用 `Download` 进行并发分段下载的功能。
- **多部分表单**: 验证了使用 `MPost` 上传文件和表单数据的正确性。

## 性能测试结果 (Benchmark)
```
goos: darwin
goarch: amd64
pkg: apigo.cc/go/http
cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
BenchmarkGet-16            15944             73379 ns/op
```
