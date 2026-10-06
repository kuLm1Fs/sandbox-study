# Phase 2 学习报告

> 阶段：Linux 隔离与安全原语 ｜ 周期：2026-10-03 ~ 2026-10-06 ｜ 状态：✅ 完成

## 交付物

**mini-container v2.0**（Go，447 行，`phase-2-container-v2/impl/`）：从零写的容器，`runc init` 的平替实现。

| 文件 | 职责 |
|---|---|
| `impl/main.go` | CLI 分发；`run`（宿主机侧：clone 建 namespace + cgroup + 接线）/ `child`（容器内：setns + pivot_root + seccomp + exec） |
| `impl/seccomp.go` | BPF 黑名单过滤器（`PR_SET_NO_NEW_PRIVS` → `PR_SET_SECCOMP`），6 个危险调用 → EPERM |
| `impl/network.go` | netns/veth/NAT 的编排与回收；`setns` 进预建 netns；默认网卡从路由表解析 |

**三层隔离（交付说明见 `README.md`）**

| 层 | 机制 | 防哪一类威胁 |
|---|---|---|
| 资源 | cgroup v2 `memory.max` / `pids.max` | 耗竭型：内存炸弹、fork 炸弹拖垮宿主机 |
| 系统调用 | seccomp BPF 黑名单：`mount/umount2/pivot_root/reboot/kexec_load/acct` | 逃逸与破坏：再换根、挂东西、重启宿主、换内核 |
| 网络 | netns + veth + `MASQUERADE` | 横向移动与暴露：容器看不见也碰不到宿主机网络栈 |

基座（Phase 0 继承）：PID / UTS / mount / IPC namespace —— 容器里 `PID 1` 是自己、`hostname` 独立、挂载表独立。

## 测试状态（`tests/TESTS.md`）

| 编号 | 结果 |
|---|---|
| T2-1 手建 cgroup 限内存触发 OOM | ✅ 通过（`memory.peak` 精确 = 100 MiB、`oom_kill 1`） |
| T2-2 容器内跑内存炸弹，宿主机不受影响 | ✅ 通过（内核日志 `constraint=CONSTRAINT_MEMCG`、`oom_memcg=/mini-container`） |
| T2-3 容器内 `mount` 返回 EPERM | ✅ 通过（`strace` 实证：初始化 3 次 mount = 0，容器内 mount = -1 EPERM） |
| T2-4 overlayfs copy-up / whiteout | ✅ 通过（`upper/file` 出现副本；`upper/only-lower` 是 `c 0,0` 字符设备） |
| T2-5 netns + veth + NAT 出外网 | ✅ 通过（容器内 `ping 8.8.8.8` 0% loss；宿主机看不到容器 IP；退出后 0/0/0/0 回收） |

## 知识点掌握（`知识点.md` K2-1~K2-7）

- 主线 5 个：✅ K2-1（cgroup v2 操作）、K2-2（宿主机视角 PID + 为何父进程做）、K2-3（seccomp 黑名单）、K2-5（overlayfs 三目录）、K2-7（netns + veth + NAT）
- 支线 2 个：✅ K2-4（`MS_PRIVATE` / `pivot_root` 卸旧根 / `CLONE_NEWIPC+NEWNET` 各防什么）、K2-6（`.wh.<name>` 空文件 ↔ `0:0` 字符设备，两种 whiteout 表示）

## 错题统计

[错题本](../phase-2-container-v2/mistakes/错题本.md) 共 **11 条**，全部来自实机。按类型：

| 类型 | 条数 | 代表 |
|---|---|---|
| 顺序 / 时机类（最贵） | 3 | #1 seccomp 装早了把自己的 `mount` 初始化挡掉；#3 `cg()` 在 chroot 之后调 |
| 概念混淆类 | 2 | #9 `ip link add` vs `ip addr add`；#8 `veth-h@if5` 当成网卡名 |
| 诊断方法类 | 1 | #7 NAT `-o` 填成内网口 → **`pkts=0` 就是"条件从没匹配"的铁证** |
| 流程 / 工具类 | 3 | #11 没 `git pull` 就重编，"改了代码没生效"；#6 仓库里 `go build` 产出同名二进制；#10 iptables 漏 `-A` |
| 抄教程类 | 1 | #4 Liz Rice 教程的占位符与作者本地路径直接抄过来 |
| 隔离缺陷类 | 1 | #5 挂载传播回宿主机（缺 `MS_PRIVATE`） |

**最贵的一条**：#1 —— seccomp 不可逆，装早了会把自己的初始化也挡掉。教训：**不可逆操作放最后；初始化和业务要用不同的权限阶段。**

## 方法复盘

**有效的**

- **"先手工跑一遍，再写代码"**：网络那节最典型——`ip netns` / `veth` / `MASQUERADE` 先手敲通（含踩坑），写进 `network.go` 时几乎是逐条翻译。
- **`strace` 作为"让内核说话"的工具**：`child()` 的每一行 `unix.Xxx` 都能在 strace 输出里找到对应的一行，这比讲一百句"系统调用是什么"管用。
- **真实场景暴露真实 bug**：Phase 1 的 4 层镜像暴露了 `prepareTarget`；Phase 2 的手工实验暴露了 NAT `-o` 和 `pkts=0` 的诊断法。
- **报错带上下文**：`ip()` / `iptables()` 包装函数把整条命令打进错误信息，`-A` 漏掉那次一眼就定位。

**待改进**

- **我（AI 侧）推进太快**：Session 2-3 时一次性给了大量 `syscall` / `unix.*` 代码，学习者反馈"一个都看不懂"。**教训：概念课必须先行于代码课**。补救方式（strace 逐行对表）反而成了本章最好的一段材料——以后默认先给这张表。
- **文档与代码的设计不一致**：Session 2-5 讲义示范 bridge（多容器），`impl` 用的是直连 veth（单容器）。已补说明，但**讲义和实现应当同源**。
- 环境漂移：Phase 2 在云端 VPS（EC2，无嵌套虚拟化）做，Phase 3 必须回本地 Lima —— 每章开头先确认环境可用（`入口自测` 已覆盖）。

## 下一步

→ **Phase 3（虚拟化与轻量沙盒：KVM / Firecracker / gVisor / Kata）**。
⚠️ 需 `/dev/kvm`：**回本地 Lima sandbox VM** 做（EC2 无嵌套虚拟化）。Phase 2 的云端 VPS 到此为止。

Phase 3 会用到 Phase 2 的什么：namespace/cgroup 是"共享内核隔离"的边界——Kata/gVisor 正是在这条边界上做文章（gVisor 用用户态内核换掉 syscall，Kata 直接换成一个 VM）。**Phase 2 的 seccomp 那层，就是 gVisor 要接管的那一层。**
