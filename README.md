# sandbox-study · 沙盒平台学习路线

面向沙盒平台工程师方向的动手学习项目（容器运行时 → Linux 隔离原语 → 轻量虚拟化 → K8s 编排 → 沙盒平台 Capstone）。

**路线**：容器运行时进阶 → Linux 隔离与安全原语 → 轻量虚拟化 → K8s 编排与调度 → 沙盒平台 Capstone

完整路线（含每个 session 的动手步骤、材料链接、验收标准）：[docs/学习路线.md](docs/学习路线.md)

## 学习协议（ADHD 友好版）

1. 一个 session 只做一件事，25–40 分钟硬停
2. 先动手，后看文档，不预习
3. 做完打卡 ✅，卡住写一句卡在哪 ⏸️
4. 不囤教程：材料只看路线里列的那一个

## 进度打卡

> 每个阶段开始前先做 [docs/入口自测.md](docs/入口自测.md)（15 分钟）：全过可压缩或跳过该阶段。

- [x] Phase 0：容器基础（已完成，只做入口自测）
- [ ] Phase 1：容器运行时进阶 → [`phase-1-mini-oci/`](phase-1-mini-oci/)
- [ ] Phase 2：Linux 隔离与安全原语 → [`phase-2-container-v2/`](phase-2-container-v2/)
- [ ] Phase 3：虚拟化与轻量沙盒 → [`phase-3-sandbox-compare/`](phase-3-sandbox-compare/)
- [ ] Phase 4：编排与调度 → [`phase-4-multi-runtime/`](phase-4-multi-runtime/)
- [ ] Phase 5：沙盒平台 Capstone → [`phase-5-mini-sandbox/`](phase-5-mini-sandbox/)

## 环境

- 主力机：Mac（M3/M4）；实验机：Lima 起的 ARM64 Linux VM（嵌套虚拟化开 `/dev/kvm`）
- Linux VM 里：Docker、Go 1.22+、git、curl；Mac 本机：Lima
- 详见 [docs/学习路线.md](docs/学习路线.md) 第三节「环境准备」
