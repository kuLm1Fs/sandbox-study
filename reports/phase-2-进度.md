# Phase 2 进度报告

> 阶段：Linux 隔离与安全原语 ｜ 开始：2026-10-03 ｜ 状态：🔄 进行中（3/5 sessions）

## 已完成

| Session | 内容 | 关键产出 |
|---|---|---|
| 2-1 ✅ | cgroup v2 手动 OOM | 手建 cgroup、设 `memory.max=100M`、看 OOM killer 干活；信号等待替代 `select{}` |
| 2-2 ✅ | 自制容器加 cgroup 限额 | `-memory`/`-pids` 参数化；失败保留现场；容器内内存炸弹被 cgroup OOM 杀掉，宿主机不受影响（T2-2） |
| 2-3 ✅ | seccomp 黑名单 | `impl/seccomp.go`：BPF 过滤器 + `PR_SET_NO_NEW_PRIVS`；`mount/umount2/pivot_root/reboot/kexec_load/acct → EPERM`；strace 实证（T2-3） |

**mini-container**（Go，296 行，`phase-2-container-v2/impl/`）：从零写的容器，`run`（父进程：clone + 建 cgroup）/ `child`（容器内：挂载 + pivot_root + seccomp + exec）双角色，`runc init` 的平替实现。

## 待完成

| Session | 内容 | 状态 |
|---|---|---|
| 2-4 ⏳ | overlayfs | 亲手 `mount -t overlay`；对照 mini-oci `unpack.go` 的 whiteout/opaque |
| 2-5 🔄 | netns + veth + NAT 接进容器 | `impl/network.go`：宿主机侧建 netns/veth/路由/NAT（对应手敲步骤）；容器侧 `setns` 进预建 netns，替掉 `CLONE_NEWNET`（空盒子问题）；待 VPS 实机跑 `ping 8.8.8.8` 验证 |
| 📦 v2.0 | 自制容器 v2.0 跑通 + README | 三层隔离各防什么写清楚 |

## 知识点掌握（`知识点.md` K2-1~K2-7）

- ✅ K2-1（cgroup v2 操作）、K2-2（宿主机视角 PID）、K2-3（seccomp 黑名单）
- ✅ K2-4（支线：MS_PRIVATE / pivot_root 卸旧根 / NEWIPC+NEWNET，在修基座时覆盖）
- ⏳ K2-5（overlayfs）、K2-7（netns）、K2-6（支线：overlayfs vs 手搓 whiteout 对照）

## 错题统计

[错题本](../phase-2-container-v2/mistakes/错题本.md) 共 6 条，全部来自实机提交记录。最贵的一条：#1 seccomp 装早了把自己的 mount 初始化挡掉——不可逆操作放最后。

## 下一步

→ Session 2-4（overlayfs）。做完 2-4/2-5 后写 Phase 2 学习报告归档。
