# Phase 4 项目：multi-runtime 集群

**目标**：单节点 k3s/kind + 两个 RuntimeClass（runc 默认 + runsc），验证多 runtime 共存。

## 内容

- RuntimeClass 配置（`handler: runsc`）
- README 说明：如何接入一个新 runtime（以 kata 为例写步骤）

## 验收

- 同一集群里 runc 和 runsc 的 pod 共存
- 能画出 kubectl → apiserver → etcd / scheduler → kubelet → containerd → runc 调用链

## 进度

- [ ] Session 4-1 ~ 4-6（见 docs/学习路线.md）
- [ ] 多 runtime 集群跑通
