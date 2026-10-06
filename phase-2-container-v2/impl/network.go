package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

// 容器网络的编排参数。用独立网段，避免和宿主机 / 其他实验冲突。
const (
	netnsName = "mini-container-net" // netns 的名字（就是 /var/run/netns/ 下的文件名）
	hostVeth  = "veth-mc-h"          // 网线在宿主机那头
	ctrVeth   = "veth-mc-c"          // 网线在容器那头
	hostAddr  = "10.200.9.1"         // 宿主机这头的地址（也是容器的网关）
	ctrAddr   = "10.200.9.2"         // 容器这头的地址
	netCIDR   = "10.200.9.0/24"      // 整个网段（NAT 规则要按它限定来源）
)

// ip 执行 `ip` 命令来编排网络。
//
// 为什么用命令而不是 netlink 系统调用？因为这里做的事**和你手敲的完全一样**：
// 建 netns、拉 veth、配 IP、加路由。真 CNI 插件（containerd/calico 调用的那个）也是
// 这套动作，只是它用 Go 库直接发 netlink 消息。先用命令把"要做哪些事"弄明白。
func ip(args ...string) error {
	out, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// setupNetwork 在**宿主机**上把网络准备好：建 netns、拉网线、配 IP 和路由。
//
// 全部发生在容器进程启动**之前** —— 这样父子进程之间就不需要同步等待了。
// 先 cleanupNetwork 是为了幂等：上次异常退出留下的残留先清掉。
func setupNetwork() error {
	cleanupNetwork()

	steps := [][]string{
		{"netns", "add", netnsName},
		// 一根虚拟网线，两头都有名字
		{"link", "add", hostVeth, "type", "veth", "peer", "name", ctrVeth},
		// 把容器那头"拔下来插进" netns 里（移进去之后宿主机就看不见它了）
		{"link", "set", ctrVeth, "netns", netnsName},
		// 宿主机这头：地址 + 起网卡（网卡默认是 DOWN 的）
		{"addr", "add", hostAddr + "/24", "dev", hostVeth},
		{"link", "set", hostVeth, "up"},
		// 容器这头：用 ip netns exec 在那个网络栈里执行
		{"netns", "exec", netnsName, "ip", "addr", "add", ctrAddr + "/24", "dev", ctrVeth},
		{"netns", "exec", netnsName, "ip", "link", "set", ctrVeth, "up"},
		{"netns", "exec", netnsName, "ip", "link", "set", "lo", "up"},
		// 指路牌：不认识的地址，都交给宿主机那头
		{"netns", "exec", netnsName, "ip", "route", "add", "default", "via", hostAddr},
	}
	for _, s := range steps {
		if err := ip(s...); err != nil {
			return err
		}
	}
	fmt.Printf("网络就绪: %s(%s) <--veth--> %s(%s)\n", hostVeth, hostAddr, ctrVeth, ctrAddr)
	return nil
}

// enableNAT 让容器能借用宿主机的身份出公网。
// 两件事：开转发（宿主机愿意当路由器）+ 对"来源是本网段"的包做 MASQUERADE。
// 只碰 NAT 表，绝不改 FORWARD 的默认策略 —— 所以不会影响 SSH。
func enableNAT() error {
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1"), 0o644); err != nil {
		return fmt.Errorf("开 ip_forward: %w", err)
	}
	iface, err := defaultIface()
	if err != nil {
		return err
	}
	if err := iptables("-t", "nat", "POSTROUTING", "-s", netCIDR, "-o", iface, "-j", "MASQUERADE"); err != nil {
		return err
	}
	fmt.Printf("NAT: %s → %s (MASQUERADE)\n", netCIDR, iface)
	return nil
}

// defaultIface 查默认路由走哪张网卡。
// 别把 ens5/eth0 写死 —— 每台机器、每张网卡都可能不一样（你刚才就差点踩这个）。
func defaultIface() (string, error) {
	out, err := exec.Command("ip", "-o", "route", "show", "default").Output()
	if err != nil {
		return "", fmt.Errorf("查默认路由: %w", err)
	}
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("找不到默认路由网卡: %q", strings.TrimSpace(string(out)))
}

func iptables(args ...string) error {
	out, err := exec.Command("iptables", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// cleanupNetwork 撤掉网络改动。容器退出后调用。
// 出错只忽略：netns 不存在、规则不存在都算"已经干净了"。
func cleanupNetwork() {
	_ = ip("netns", "del", netnsName) // 删 netns 会连带删掉里面的 veth-c
	if iface, err := defaultIface(); err == nil {
		_ = iptables("-t", "nat", "-D", "POSTROUTING", "-s", netCIDR, "-o", iface, "-j", "MASQUERADE")
	}
}

// joinNetns 在**容器进程里**执行：加入那个预先建好的 netns。
//
// 两个必须注意的点：
//  1. namespace 的归属是**每线程**的 —— 所以要在 runtime.LockOSThread() 之后、
//     execve 之前调用（我们 child() 里就是这个顺序）。
//  2. 必须在 pivot_root **之前**调用 —— 换根之后 /var/run/netns 就看不见了。
func joinNetns() error {
	f, err := os.Open("/var/run/netns/" + netnsName)
	if err != nil {
		return fmt.Errorf("打开 netns(%s): %w（setupNetwork 没跑成功？）", netnsName, err)
	}
	defer f.Close()
	if err := unix.Setns(int(f.Fd()), unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("setns 进 %s: %w", netnsName, err)
	}
	return nil
}
