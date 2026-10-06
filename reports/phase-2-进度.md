# Phase 2 进度报告

> 阶段：Linux 隔离与安全原语 ｜ 开始：2026-10-03 ｜ 状态：✅ 已完成（5/5 sessions + 项目）

## 已完成

| Session | 内容 | 关键产出 |
|---|---|---|
| 2-1 ✅ | cgroup v2 手动 OOM | 手建 cgroup、设 `memory.max=100M`、看 OOM killer 干活；`memory.peak` 精确停在 100 MiB |
| 2-2 ✅ | 自制容器加 cgroup 限额 | `-memory`/`-pids` 参数化；失败保留现场；容器内内存炸弹被 cgroup OOM 杀掉，宿主机不受影响（T2-2） |
| 2-3 ✅ | seccomp 黑名单 | `impl/seccomp.go`：BPF 过滤器 + `PR_SET_NO_NEW_PRIVS`；`mount/umount2/pivot_root/reboot/kexec_load/acct → EPERM`（T2-3，strace 实证） |
| 2-4 ✅ | overlayfs | 手挂 overlay：**copy-up**（改文件 → upper 出现整份副本、lower 不变）、**whiteout**（删下层文件 → upper 出现 `c 0,0` 字符设备）；多层 `lowerdir=l2:l1` = 并集 + 上层覆盖 |
| 2-5 ✅ | netns + veth + NAT | 手工搭网（两 netns 互 ping → 配 NAT 出公网）✅；`impl/network.go` 接进容器 ✅（T2-5：容器内 `ping 8.8.8.8` 0% loss） |

**mini-container**（Go，447 行，`phase-2-container-v2/impl/`）：从零写的容器，`run`（父进程：clone + cgroup + 接线）/ `child`（容器内：setns + pivot_root + seccomp + exec）双角色，`runc init` 的平替实现。三个源文件分工：`main.go`（流程）/ `seccomp.go`（BPF 过滤器）/ `network.go`（netns/veth/NAT 编排与回收）。

## 三层隔离（交付说明见 `phase-2-container-v2/README.md`）

| 层 | 机制 | 防什么 |
|---|---|---|
| 资源 | cgroup v2 `memory.max`/`pids.max` | 耗竭型：内存炸弹、fork 炸弹拖垮宿主机 |
| 系统调用 | seccomp BPF 黑名单（6 个调用 → EPERM） | 逃逸与破坏：再换根、挂东西、重启宿主、换内核 |
| 网络 | netns + veth + MASQUERADE | 横向移动：容器看不见也碰不到宿主机的网络栈 |

（另有 namespace 基座：PID / UTS / mount / IPC —— Phase 0 继承，容器里 `PID 1` 是自己、`hostname` 独立、挂载表独立。）

## 知识点掌握（`知识点.md` K2-1~K2-7）

主线 5 个全部 ✅：K2-1（cgroup v2 操作）、K2-2（宿主机视角 PID）、K2-3（seccomp 黑名单）、K2-5（overlayfs）、K2-7（netns + veth + NAT）
支线 2 个全部 ✅：K2-4（MS_PRIVATE / pivot_root 卸旧根 / NEWIPC+NEWNET）、K2-6（`.wh.` ↔ `c 0,0` 的两种 whiteout 表示）

## 错题统计

[错题本](../phase-2-container-v2/mistakes/错题本.md) 共 **11 条**，全部来自实机。最贵的三条：

- **#1 seccomp 装早了把自己的 mount 初始化挡掉** —— 不可逆操作必须放最后。
- **#7 NAT 的 `-o` 填成内网口** —— `pkts=0` 是"条件从没匹配"的铁证，先怀疑接口名/方向。
- **#11 改了代码没生效：忘了 `git pull` 就重编** —— "我明明改了"的头号原因是"跑的不是新代码"。

## 关键实证（可复现的"证据链"）

```bash
# 资源：内存炸弹撞上限
sudo work/mini-container -memory 100M run /membomb
#   → memory.peak=104857600、oom_kill 1、signal: killed
#   → 内核日志 Memory cgroup out of memory: ... anon-rss:102140kB, constraint=CONSTRAINT_MEMCG

# 系统调用：容器内 mount 被挡
sudo strace -f -e trace=mount work/mini-container -memory 100M run /bin/sh -c "mount -t tmpfs t /tmp"
#   → 初始化那 3 次 mount = 0；容器里那次 = -1 EPERM

# 网络：隔离 + 出网
sudo work/mini-container -memory 100M run /bin/sh    # 容器内：ping -c3 8.8.8.8 → 0% loss
ip addr | grep -c 10.200.9.2                          # → 0（宿主机看不到容器 IP）
ip netns list; sudo iptables -t nat -L POSTROUTING -n | grep -c MASQUERADE   # → 0 / 0（回收干净）
```

## 下一步

→ 本 phase 归档完成。可进 **Phase 3（虚拟化与轻量沙盒：KVM / Firecracker / gVisor / Kata）**。
注意 Phase 3 需要 `/dev/kvm`，**回本地 Lima sandbox VM**（EC2 没有嵌套虚拟化）；Phase 2 用的这台 VPS 到此为止。
