# Phase 4 测试用例

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T4-1 | 纸上画出调用链 | `kubectl` → apiserver → etcd / scheduler → kubelet → containerd → runc，一环不缺 |
| T4-2 | 口头：CNI 谁调、何时调、IPAM 干什么 | 答出：kubelet 经 containerd 在创建 Pod 网络时调用；IPAM 分配 Pod IP |
| T4-3 | apply 带 `resources.requests/limits` 的 deployment，`kubectl describe node` | 能看到 allocatable / 已分配资源；超限 pod 被驱逐或 pending |
| T4-4 | apply `runtimeclass-runsc.yaml`，分别跑 runc 和 runsc 的 pod | 同一集群两种 pod 共存；runsc 的 pod 里 `dmesg` 显示 gVisor |

**备注**：T4-1 画不出 → 回 Session 4-1~4-4 过一遍组件交互；环境太重 → 用 kind 单节点先跑通概念。
