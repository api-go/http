# AI 指导规则 (AI.md)

## 模块信息
- **模块名**: `http`
- **当前版本**: `v1.1.0`
- **核心意图**: 高性能 HTTP 客户端，支持泛型绑定、并行分段下载与线程安全的全局 Header 管理。

## AI 调用建议
- **初始化**: 优先使用 `NewClient(timeout)`。
- **结果解析**: 
    - 结构化数据：使用 `Bind[T](result)`。
    - 列表数据：使用 `result.Slice()` 或 `Bind[[]T](result)`。
    - 键值数据：使用 `result.Map()` 或 `Bind[map[string]T](result)`。
- **并行下载**: 使用 `Download`，可通过 `client.MaxConnsPerHost` 调节并发。
- **表单提交**: 使用 `PostMultipart` 处理带文件的多部分表单。
- **全局配置**: 使用 `SetGlobalHeader(k, v)` 设置线程安全的全局头。

## 注意事项
- **并发安全**: `Client` 的方法是并发安全的，但 `Result` 对象不是。
- **内存池**: 内部使用 `sync.Pool` 优化了 Buffer 分配，在大负载下表现优异。
- **Body 释放**: `Bind`, `String`, `Bytes`, `Map`, `Slice` 均会自动关闭 Body。`ManualDo` 需要手动关闭。
