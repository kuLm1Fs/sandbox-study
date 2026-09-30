# mini-sandbox 实现（Go）

对应路线：`docs/学习路线.md` Phase 5（Capstone）。

## 要实现的模块

- `Runtime` 抽象：`ContainerRuntime(runc)` + `MicroVMRuntime(firecracker)`，统一 `create/start/stop/destroy` 接口
- 并发任务池：goroutine worker pool，记录成功率/耗时
- 环境模板：YAML 定义环境（镜像/kvm 配置/验收脚本），`verify` 命令跑验收
- 网络：每个沙盒独立 netns；egress 代理；连通性自检
- 生命周期：`snapshot`/`restore`（容器用 overlay upper 层打包，VM 用 virsh 快照）；JSONL 操作日志；`replay` 复现

## 运行

```bash
cd phase-5-mini-sandbox/impl
go run . run --runtime firecracker --template demo-task
go run . replay <task-id>
```

## 里程碑

见 `../README.md`（W1–W6）。
