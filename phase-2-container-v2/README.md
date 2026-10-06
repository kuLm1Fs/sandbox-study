# Phase 2 项目：自制容器 v2.0（Go）

**目标**：在 Phase 0 的 container-from-scratch 基础上加三层隔离（建议用 Go 重写或封装）。

## 目录结构

- `impl/`：Go 实现（`go.mod` + 源码）
- `tests/`：测试用例（`TESTS.md`），含 cgroup / seccomp / 网络的验证步骤

## 要加的三层

1. cgroup v2 资源限制（CPU / 内存）
2. seccomp profile（默认禁用 mount、reboot 等危险调用）
3. 网络（veth + NAT，复用 netns 实验脚本）

## 验收

- v2.0 跑通，容器里跑内存炸弹不影响宿主机
- README 写清三层隔离分别防什么

## 进度

- [x] Session 2-1｜cgroups v2 动手（手建 cgroup 限内存、看 OOM killer 干活）
- [x] Session 2-2｜自制容器加 cgroup 限额（`-memory`/`-pids`）；容器内内存炸弹被 cgroup OOM 杀掉，宿主机不受影响
- [x] Session 2-3｜seccomp（容器内 `mount` 返回 EPERM；`/proc/self/status` 里 `Seccomp: 2`）
- [ ] Session 2-4｜overlayfs
- [ ] Session 2-5｜netns + veth + NAT
- [ ] v2.0 跑通 + README
