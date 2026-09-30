# Phase 5 测试用例

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T5-1 | `go run ./impl run --runtime runc --template demo-task` | 任务成功（exit 0）；JSONL 日志有完整记录 |
| T5-2 | `go run ./impl run --runtime firecracker --template demo-task` | 任务成功；同一接口不同后端行为一致 |
| T5-3 | 构造一个失败任务，记下 task-id，`go run ./impl replay <task-id>` | 复现出相同的失败输出 |
| T5-4 | `snapshot` 后改环境，再 `restore` | 恢复后环境与快照点一致 |
| T5-5 | 跑连通性自检 | 沙盒 netns 与宿主机隔离；egress 代理可达外网 |
| T5-6 | 并发压测（N=20 个任务） | 输出成功率/平均耗时；无任务串扰（网络/文件隔离） |

**备注**：T5-2 失败 → 先确认 Phase 3 的 Firecracker 已跑通；T5-6 有串扰 → 检查 netns 和端口分配逻辑。
