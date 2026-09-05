# CHANGELOG

## v1.5.7 (2026-09-05)
- **依赖对齐**: 升级 `config`、`file` 与 `log` 至当前已发布版本。

## v1.5.6 (2026-09-05)
- **依赖对齐**: 升级全部 `apigo.cc/go/*` 依赖至当前已发布版本。

## v1.5.5 (2026-09-05)
- **WebSocket 客户端**: 基于 `github.com/gorilla/websocket v1.5.3` 新增包级与客户端级 `WebSocket` 连接能力，复用客户端超时、Cookie Jar 和全局 Header，并提供文本、二进制及 JSON 消息辅助方法。
- **JS 支持**: `http.WebSocket` 与 `http.New(...).WebSocket` 可在低代码中建立和操作 WebSocket 连接。
- **依赖更新**: 对齐全部 `apigo.cc/go/*` 依赖至模块索引中的当前版本。

## v1.5.4 (2026-07-18)
- **依赖校验修复**: 更新 `go.sum` 为当前可验证的依赖 checksum。

## v1.5.3 (2026-06-21)
- **JS 对齐**: 重构 JS 导出为具名函数，并引入 `jsmod.MakeError` 动态包装错误以获取调用栈。
- **依赖更新**: 升级依赖 `jsmod` 至 `v1.5.3`，`cast` 至 `v1.5.3`，`rand` 至 `v1.5.3`，`encoding` 至 `v1.5.4`，`shell` 至 `v1.5.3`，`safe` 至 `v1.5.2`，`id` 至 `v1.5.4`，`file` 至 `v1.5.5`，`config` 至 `v1.5.3`，`log` 至 `v1.5.8`。

## v1.5.1 (2026-06-08)
- **JS 对齐 & 安全加固**:
    - HTTP 方法名统一更正为全大写（`GET`, `POST`, `PUT`, `DELETE`），强化协议语义。
    - **Headers 优化**: JS 侧 Headers 参数从变长字符串改为更符合 JS 习惯的 `map[string]string` 对象。
    - **深度安全扫描**: 在 `POST/PUT` 请求中对 `Multipart` 涉及的文件路径进行前置扫描，强制通过 `file.VerifyPathForSafeMode` 校验，严防低代码环境下的敏感文件泄露。
    - **结果集对齐**: `Result.Save` 方法现在同样受安全沙箱控制。

## v1.3.4 (2026-05-30)
- **API 变更**: 将 `timeout(ms)` 拆分为 `new(ms)` 和 `newH2C(ms)` 以支持 HTTP/2 Cleartext。
- **安全性**: 移除 `setGlobalHeader` / `getGlobalHeader` 以增强脚本间隔离。

## v1.3.3 (2026-05-30)
- **新增**: 注册到 `jsmod`。
- **优化 (JSMOD)**: 提供 `timeout(ms)` 支持动态超时控制，封装 `jsResult` 以屏蔽敏感的 `Save()` 方法，支持 `form()` 和 `multipart()` 数据标记。

## v1.0.10 (2026-05-09)
- **CI/CD 对齐**:
    - 修复测试用例由于默认绑定泛地址导致的 `connection refused` 错误，显式绑定本地测试服务器至 `127.0.0.1`，提高 macOS 与多网卡环境下的测试稳定性。
    - 更新 `TEST.md`，同步最新的测试用例覆盖场景（包括 `Multipart` 和 `Form` 的验证）及真实 Benchmark。

## v1.0.9 (2026-05-09)
- **基础设施对齐**: 
    - 升级 `apigo.cc/go/log` 至 `v1.1.13`。
    - 确保与 `log` 包最新的“绝对索引”和“强制 Reset 契约”兼容。`http` 包不涉及自定义日志类型，故无代码变更。

## v1.0.8 (2026-05-08)
- **性能优化 (Memory Efficiency)**:
    - 重构 `Multipart` 处理逻辑，采用 `io.Pipe` 实现流式上传，彻底解决大文件上传时的内存撑爆问题。
    - 引入 `writeMultipartPart` 与 `writeMultipartFile` 辅助函数，优化分块写入逻辑。
- **功能增强 (Unified Type Handling)**:
    - 统一 `do` 方法内部逻辑，通过类型分组消除冗余代码。
    - 将 `map[string][]any` 提升为 `Multipart` 处理，确保复杂/混合数据类型的正确流式发送。
    - 完善对 `url.Values`, `map[string][]string`, `Form` 的统一表单编码支持。
    - 优化 `Multipart` 对多值字段的支持，允许在 `Multipart` 映射中使用 `[]string` 或 `[]any` 发送同名多参数。
- **基础设施对齐**: 
    - 更新 `go/encoding` 至 v1.0.6。
    - 移除 `bufferPool` 在 `Multipart` 中的使用，转向更高效的流式 IO。

## v1.0.6 (2026-05-07)
- 内部版本，优化 Header 自动透传逻辑。

## v1.0.5 (2026-05-06)
- 内部版本，增强对 H2C 场景下的连接池管理。

## v1.0.4 (2026-05-05)
- **基础设施对齐**: 
    - 更新 `go/file` 至 v1.0.5, `go/log` 至 v1.1.1, `go/config` 至 v1.0.5。
    - `PostMultipart` 现在通过 `go/file` API 透明支持物理文件与内存文件系统的读取。
- **鲁棒性增强**: 
    - 优化 `doByRequest` 中对 `X-Forwarded-For` 的处理，增强 `RemoteAddr` 解析的健壮性。
- **代码规范**: 
    - 统一变量命名规范，重命名冗余的单字母变量，提升代码可读性。

## v1.0.3 (2026-05-03)
- **API 变更**: 将泛型解析函数 `Bind[T]` 重命名为 `To[T]`，以保持与全局 API 风格一致。
- **文档优化**: 移除冗余的 `AI.md`，更新 `README.md` 中的 API 示例。

## v1.0.2 (2026-05-03)
- **Breaking Changes & API Renames**:
    - `ToT` 重命名为 `Bind` (泛型解析)。
    - `Result.Arr` 重命名为 `Result.Slice`。
    - `MPost` 重命名为 `PostMultipart`。
    - `GlobalHeaders` 字段私有化，改为通过 `SetGlobalHeader` / `GetGlobalHeader` 进行线程安全操作。
- **New Features**:
    - **并行下载**: `Download` 方法现在支持并行分段下载，可通过 `client.MaxConnsPerHost` 控制并发度。
    - **性能优化**: 引入 `sync.Pool` 复用 `bytes.Buffer`，降低高并发下的内存分配开销。
- **Improvements**:
    - 优化 `PostMultipart` 实现，支持更清晰的错误处理。
    - 统一内部 Header 透传逻辑，采用更高效的 slice 追加方式。

## v1.0.1 (2026-05-03)
- 重构 `Download` 逻辑，使用 `offsetWriter` 解决重试场景下的数据偏移风险。
- 迁移测试至 `http_test` 独立包。

## v1.0.0 (2026-05-02)
- 从 `github.com/ssgo/httpclient` 迁移完成。
