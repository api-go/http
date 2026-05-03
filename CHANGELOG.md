# CHANGELOG

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
