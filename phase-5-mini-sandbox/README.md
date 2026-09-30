# Phase 5 项目：mini-sandbox（Capstone ⭐）

**目标**：单机沙盒任务平台，对标 JD 全部 5 条职责，可写进简历。建议用 Go 实现（Python 做过原型后重写）。

## 模块

| JD 职责 | 模块 |
|---|---|
| 大规模沙盒平台 | 并发任务池 |
| VM/容器统一接入 | `Runtime` 抽象：`ContainerRuntime(runc)` + `MicroVMRuntime(firecracker)`，统一 `create/start/stop/destroy` 接口 |
| 多任务环境适配 | YAML 环境模板 + `verify` 验收命令 |
| 网络代理与隔离 | 独立 netns + egress 代理 + 连通性自检 |
| 生命周期管理 | snapshot/restore + JSONL 操作日志 + `replay` 回放 |

## 验收

1. 一条命令起任务（如 `minisandbox run --runtime firecracker --template python-task`）
2. 任务失败后 `replay` 能复现
3. README 有架构图 + 对标 JD 的 5 条映射表

## 里程碑

- [ ] W1：Runtime 抽象 + runc 后端
- [ ] W2：firecracker 后端
- [ ] W3：镜像管理（复用 mini-oci）+ 环境模板
- [ ] W4：快照/恢复 + 日志回放
- [ ] W5：网络隔离 + egress 代理
- [ ] W6：并发压测 + README + demo
