# Phase 3 测试用例

在 Linux VM 里执行（需要 `/dev/kvm`）。

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T3-1 | `virt-install` 起一台 VM；`virsh snapshot-create-as` 打快照再恢复 | `virsh list` 看到 VM；快照恢复后 VM 状态回到快照点 |
| T3-2 | 跟 Firecracker getting-started 跑通 microVM，ssh 进去跑 `uname -r` | 能进 microVM；内核版本与宿主机**不同** |
| T3-3 | `ctr run --runtime io.containerd.runsc.v1` 跑 busybox，进容器跑 `dmesg` | 容器运行；dmesg 有 gVisor 启动日志 |
| T3-4 | apply `runtimeClassName: kata` 的 pod，`kubectl logs` / exec 跑 `uname -r` | pod 运行；显示的是 guest 内核版本 |
| T3-5 | `go run ./impl` | 输出三方案启动耗时中位数 + 内存开销；RESULTS.md 含对比表和 3 条结论 |

**备注**：T3-2 网络那节卡住 → 先跳过，只要进 serial console 就算过；`/dev/kvm` 不存在 → 回路线文档第三节检查 Lima 嵌套虚拟化配置。
