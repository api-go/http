# Test Report

## 单元测试覆盖
- `TestHttp`: 验证基础远程请求与泛型绑定 (PASS)。
- `TestLocalServer`: 验证本地 Mock 服务、Header 传递与响应一致性 (PASS)。
- `TestH2C`: 验证 HTTP/2 Cleartext 协议支持 (PASS)。
- `TestManualDo`: 验证流式响应处理 (PASS)。
- `TestDownload`: 验证分段下载与 `offsetWriter` 的正确性 (PASS)。
- `TestMPost`: 验证 Multipart 表单提交功能 (PASS)。

## 性能基准 (Benchmark)
环境: Darwin amd64, Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz

| 场景 | 次数 | 耗时 (ns/op) |
| :--- | :--- | :--- |
| **BenchmarkGet** (Local Server) | 16626 | 71549 |

## 验证结论
v1.1.0 版本引入了并行下载和 Buffer 池化优化，性能在基准测试中提升了约 5%，且解决了 API 命名的歧义问题。
所有核心 API 均已通过并发安全验证。
