# CHANGELOG

## v1.0.0 (2026-05-02)
- 从 `github.com/ssgo/httpclient` 迁移完成。
- **Breaking Changes**:
    - 包名变更为 `apigo.cc/go/http`。
    - `ClientPool` 重命名为 `Client`。
    - 移除对 `ssgo/standard` 的依赖，内置 Header 常量。
    - `Result.To` 内部集成 `cast` 和 `convert` 的智能映射逻辑。
- **New Features**:
    - 新增泛型解析函数 `ToT[T](*Result)`。
    - 所有文件 IO 逻辑自动支持目录创建 (`EnsureParentDir`)。
    - 请求 ID 自动生成策略升级。
