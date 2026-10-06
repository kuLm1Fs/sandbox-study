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
- [ ] Session 2-5｜netns + veth + NAT（👇 完整讲义在下面）
- [ ] v2.0 跑通 + README

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

（待实机补充——这一节你先跑，踩到报错贴回来，我按"看到什么 → 为什么 → 怎么修"收进错题本。）

**下一步**：→ Session 2-4（overlayfs），然后合起来做 v2.0（cgroup + seccomp + 网络三层隔离）。
