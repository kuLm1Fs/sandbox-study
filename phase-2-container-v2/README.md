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
- [x] Session 2-4｜overlayfs（手挂 overlay，拿到 copy-up 与 whiteout `c 0,0`）
- [x] Session 2-5｜netns + veth + NAT（手工搭网 ✅ + 接进容器 ✅，容器内 `ping 8.8.8.8` 通）
- [x] v2.0 跑通 + README（三层隔离见下）

---

## 三层隔离：各防什么（v2.0 交付说明）

`mini-container` 把一个进程关进**三层不同种类**的笼子里。三层不是重复，而是各自堵一类威胁：

| 层 | 机制 | 防什么 | 可复现的证据 |
|---|---|---|---|
| **资源** | cgroup v2：`memory.max` / `pids.max` | **耗竭型**攻击：内存炸弹、fork 炸弹把宿主机拖垮 | 容器内跑 `/membomb` → `memory.peak` 精确停在 `104857600`（100 MiB）、`oom_kill 1`；宿主机 `free -h` 不变，内核日志 `constraint=CONSTRAINT_MEMCG` |
| **系统调用** | seccomp BPF 黑名单：`mount` / `umount2` / `pivot_root` / `reboot` / `kexec_load` / `acct` | **逃逸与破坏**：再换根、挂东西、重启宿主、换内核 | 容器内 `mount -t tmpfs t /tmp` → `EPERM`；`strace -e trace=mount` 实证 `mount(...) = -1 EPERM`，而初始化阶段那 3 次 `mount` 都 `= 0` |
| **网络** | netns + veth + `MASQUERADE` 快照隔离 | **横向移动与暴露**：容器不该看见、不该碰宿主机的网络栈 | 容器内 `ping 8.8.8.8` 通（0% loss）；宿主机 `ip addr` 里看不到 `10.200.9.2`；容器退出后 `netns`/`veth`/NAT 规则全部回收（0/0/0/0） |

基座还有一层（Phase 0 继承下来的 **namespace**）：PID / UTS / mount / IPC —— 容器里 `PID 1` 是它自己、`hostname` 独立、挂载表独立。

**为什么要三层一起**：它们**互相独立失效**。seccomp 漏了一个 syscall，cgroup 还能兜住资源；cgroup 配置写错，netns 还挡着网络；就算都破了，`pivot_root` 已经卸掉旧根，逃不出去。

### 怎么跑

```bash
# ① 前置：rootfs（用 Phase 1 的工具从真实镜像解出来，不依赖 docker）
cd ../phase-1-mini-oci
go run ./impl pull busybox:latest
go run ./impl unpack busybox:latest

# ② 编译（syscall.CLONE_* 是 Linux 专有，只能在 Linux 上编译运行）
cd ../phase-2-container-v2
go build -o work/mini-container ./impl

# ③ 跑（-memory/-pids 可调）
sudo work/mini-container -memory 100M -pids 20 run /bin/sh
```

### 代码结构

| 文件 | 职责 |
|---|---|
| `impl/main.go` | CLI 分发 + `run`（宿主机侧：clone + cgroup + 接线）/ `child`（容器内：setns + pivot_root + seccomp + exec） |
| `impl/seccomp.go` | BPF 黑名单过滤器（`PR_SET_NO_NEW_PRIVS` → `PR_SET_SECCOMP`） |
| `impl/network.go` | netns/veth/NAT 的编排与回收；`setns` 进预建 netns |

**设计要点**：接线和 cgroup 都由**父进程**在 `clone` 之前完成（父进程才有宿主机视角），子进程只负责 `setns` 进预建 netns、换根、装 seccomp、`execve`。这就是 runc 的 `runc init` 分工。

---

## Session 2-5｜宿主机侧搭网：netns + veth + NAT（35 分钟）

**在哪做**：Linux VM（要 root；本节所有命令都在**宿主机侧**执行）

**目标**：亲手搭出容器网络的宿主机侧——两个 netns 互 ping，再配 NAT 让它们上外网。做完能说出 docker0 那套东西到底是哪几条命令。

**前置自检**（复制粘贴，都有输出才往下）：

```bash
sudo -v && echo root-ok
ip netns list          # 空也行，能执行不报错就行
which iptables         # 有路径输出
```

### 动手

**第 1 步：建两个 netns（两个"隔离的网络栈"）**

```bash
sudo ip netns add ns1
sudo ip netns add ns2
ip netns list
```

预期输出：

```
ns1
ns2
```

**第 2 步：建 bridge + 两对 veth，把 netns 插到桥上**

```bash
sudo ip link add br0 type bridge
sudo ip addr add 10.200.0.1/24 dev br0
sudo ip link set br0 up
sudo ip link add veth1 type veth peer name veth1-ns
sudo ip link add veth2 type veth peer name veth2-ns
sudo ip link set veth1-ns netns ns1
sudo ip link set veth2-ns netns ns2
sudo ip link set veth1 master br0
sudo ip link set veth2 master br0
sudo ip link set veth1 up
sudo ip link set veth2 up
```

**第 3 步：进 netns 配 IP、起网卡、指默认路由**

```bash
sudo ip netns exec ns1 ip link set lo up
sudo ip netns exec ns1 ip link set veth1-ns up
sudo ip netns exec ns1 ip addr add 10.200.0.2/24 dev veth1-ns
sudo ip netns exec ns1 ip route add default via 10.200.0.1
sudo ip netns exec ns2 ip link set lo up
sudo ip netns exec ns2 ip link set veth2-ns up
sudo ip netns exec ns2 ip addr add 10.200.0.3/24 dev veth2-ns
sudo ip netns exec ns2 ip route add default via 10.200.0.1
```

**第 4 步：互 ping（隔离网络内部通了）**

```bash
sudo ip netns exec ns1 ping -c 2 10.200.0.3
```

预期输出：`2 packets transmitted, 2 received, 0% packet loss`。

顺手看一眼隔离感——宿主机上执行：

```bash
ip addr | grep "10.200.0"
```

预期：只看到 `10.200.0.1/24`（br0）和 veth 的宿主机端，**看不到** `10.200.0.2` / `10.200.0.3`——那两个 IP 只活在各自的 netns 里。

**第 5 步：开转发 + MASQUERADE，让 ns1 上外网**

```bash
echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward
sudo iptables -t nat -A POSTROUTING -s 10.200.0.0/24 -j MASQUERADE
sudo iptables -t nat -L POSTROUTING -n | grep MASQUERADE
```

预期：看到 `MASQUERADE  all  --  10.200.0.0/24  0.0.0.0/0` 一行。

```bash
sudo ip netns exec ns1 ping -c 2 8.8.8.8
```

预期：`2 received`。ping 不通先看自查清单第 3 条，不硬啃。

**第 6 步：清理（别跳过，不然 10.200.0.0/24 和 iptables 规则一直占着）**

```bash
sudo ip netns del ns1   # 删 netns 会自动带走它里面的 veth 端
sudo ip netns del ns2
sudo ip link del br0
sudo iptables -t nat -D POSTROUTING -s 10.200.0.0/24 -j MASQUERADE
ip netns list           # 空
```

### 刚才发生了什么

- `ip netns add` = 新建一个独立的网络栈（自己的网卡、IP、路由表、iptables 都独立）。
- `veth` 对 = 一根虚拟网线，两个头各插在一个网络栈里；一头扔进 netns，另一头留在宿主机。
- `bridge br0` = 二层交换机，宿主机端的两个 veth 头都插上去，ns1/ns2 就二层互通了——这就是 docker0 的原理。
- `MASQUERADE` = 一种 SNAT：把源 IP `10.200.0.x` 换成宿主机出口 IP 再发出去，回包时再换回来。容器上网 = veth + bridge + MASQUERADE 三件套。

### 验证

不看笔记回答：veth、bridge、MASQUERADE 各解决什么问题？（一根线连两个网络栈 / 二层交换让多对 veth 互通 / 源地址转换让私网 IP 上外网）

### 自查清单

```bash
ip netns list                                    # ns1 ns2 都在（清理后应为空）
sudo ip netns exec ns1 ip addr | grep 10.200.0.2 # ns1 的 IP 在里面
sudo ip netns exec ns1 ping -c 1 10.200.0.3      # 通
cat /proc/sys/net/ipv4/ip_forward                # 1
sudo iptables -t nat -L POSTROUTING -n | grep -c MASQUERADE  # 1
```

### 常见坑

（全来自实机；完整版见 [`mistakes/错题本.md`](mistakes/错题本.md)）

- **`ip link add 10.200.1.1/24 dev veth-h` → `"dev" not a valid ifname`**：把"号码"喂给了"硬件"。`ip link` 管网卡本身（创建/启停/移动），`ip addr` 管网卡上的地址。口诀：**link = 网卡，addr = 号码，route = 路牌**。
- **NAT 规则的 `-o` 填成内网口（比如 `-o veth-h`）→ 容器 ping 外网卡住不动**：`-o` 是"包从哪张网卡**出去**"，容器去公网是从 `ens5` 出去的。诊断靠包计数器：`sudo iptables -t nat -L POSTROUTING -n -v` 里这条规则 `pkts=0` = **条件从没匹配上**，先怀疑接口名/源地址/方向写错。
- **`veth-h@if5` 不是网卡名**：`@ifN` 只是 `ip` 的显示装饰（提示对端接口的 ifindex），真名是 `veth-h`。
- **`iptables -t nat POSTROUTING …` → `Bad argument 'POSTROUTING'`**：漏了 `-A`。加规则是 `-A`，删规则是 `-D`。
- **ping 卡住 vs 立刻报错，是两种病**：`Network is unreachable`（立刻）= 本机没路由，查 `ip route`；**一直卡住/超时** = 包出得去回不来，查 `ip_forward` / NAT / 回程路由。

**下一步**：→ Session 2-4（overlayfs），然后合起来做 v2.0（cgroup + seccomp + 网络三层隔离）。
