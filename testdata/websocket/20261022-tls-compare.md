# WebSocket H1/H2 TLS 对照实测（2026-10-22）

环境：Apple M3 Max，Darwin arm64，Go 1.26.4，GOMAXPROCS=16。每组 500ms、重复三次，表中为 ns/op 的中位数。两侧使用同一 TLS 测试服务、Gorilla 帧处理、不压缩、相同消息与 Send/ReadBytes 调用；只切换 H1/H2。

单流回显的 ns/op 是一次发送并接收完整回显的耗时。16 流使用 RunParallel，ns/op 是总耗时除以全部操作数，表示聚合吞吐，不能解释为单流 RTT。MB/s 只计一份有效载荷，不计回传、TLS/H2 帧头及 TCP 开销。建连 cold 每次创建客户端并关闭连接池；warm 复用客户端和已有 H2 连接，H1 每次仍需建立独立 TLS 连接。

| 场景 | 中位数 ns/op |
|---|---:|
| Compare/H1/64B/1Streams | 15092 |
| Compare/H2/64B/1Streams | 39583 |
| Compare/H1/64B/16Streams | 9627 |
| Compare/H2/64B/16Streams | 10074 |
| Compare/H1/1024B/1Streams | 19304 |
| Compare/H2/1024B/1Streams | 44054 |
| Compare/H1/1024B/16Streams | 10378 |
| Compare/H2/1024B/16Streams | 12850 |
| Compare/H1/65536B/1Streams | 156148 |
| Compare/H2/65536B/1Streams | 322134 |
| Compare/H1/65536B/16Streams | 71453 |
| Compare/H2/65536B/16Streams | 188388 |
| Handshake/H1/cold | 1564518 |
| Handshake/H2/cold | 1670491 |
| Handshake/H1/warm | 1573652 |
| Handshake/H2/warm | 106551 |

1 KiB 单流：H1 19.304 µs、H2 44.054 µs。16 流有效载荷吞吐：H1 98.67 MB/s、H2 79.69 MB/s；对应 TCP 连接分别为 16 与 1。warm 建连：H1 1.574 ms、H2 0.107 ms，H2 约快 14.8 倍。cold 建连：H1 1.565 ms、H2 1.670 ms。

结论限于本版适配实现和本机回环 TLS：H2 节省连接并显著降低后续建连成本；持续回显存在额外调度、拷贝和 HTTP/2 流控成本，当前没有吞吐优势。不能把结果外推为浏览器、广域网、高 RTT、丢包或高连接数的性能结论，也不能把内存分配 B/op 当作连接常驻内存。原始输出见 [20261022-tls-compare.txt](20261022-tls-compare.txt)。
