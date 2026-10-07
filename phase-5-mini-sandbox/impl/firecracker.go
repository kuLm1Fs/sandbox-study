package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// FirecrackerRuntime 用 firecracker microVM 实现 Runtime 合同。
// 同一个合同，第二种填法：main.go 只换一行，底下就从容器变成真虚拟机。
// 逻辑就是 3-5 的 fc-bench.sh：配（Create）→ 点火等 ssh（Start）→ 杀进程（Stop/Destroy）。
type FirecrackerRuntime struct {
	Sock   string // API socket 路径
	Bin    string // firecracker 二进制
	Kernel string // guest 内核
	Rootfs string // guest rootfs
	Tap    string // 宿主 tap 设备（3-5 建的 tap0，重启 WSL 会丢，丢了重建）
	IP     string // guest IP
	Key    string // ssh 私钥
	proc   *exec.Cmd
}

var _ Runtime = (*FirecrackerRuntime)(nil)

// NewFirecrackerRuntime 用 3-5 实机的路径给默认值。
func NewFirecrackerRuntime() *FirecrackerRuntime {
	return &FirecrackerRuntime{
		Sock:   "/tmp/fc-w1.sock",
		Bin:    "/home/kms/fc/release-v1.17.0-x86_64/firecracker-v1.17.0-x86_64",
		Kernel: "/home/kms/fc/vmlinux",
		Rootfs: "/home/kms/fc/ubuntu-24.04.ext4",
		Tap:    "tap0",
		IP:     "172.16.0.2",
		Key:    "/home/kms/fc/ubuntu-24.04.id_rsa",
	}
}

// api 用 curl 调 firecracker 的 unix socket API（跟 fc-bench.sh 一样）。
func (f *FirecrackerRuntime) api(method, path, json string) error {
	args := []string{"-s", "-X", method, "--unix-socket", f.Sock, "http://localhost" + path}
	if json != "" {
		args = append(args, "--data", json)
	}
	if out, err := exec.Command("curl", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("API %s %s 失败: %v\n%s", method, path, err, out)
	}
	return nil
}

func (f *FirecrackerRuntime) waitSock(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(f.Sock); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("socket %s %v 未出现", f.Sock, timeout)
}

func (f *FirecrackerRuntime) Create(id, image string) error {
	_ = image // firecracker 后端 rootfs 固定，image 用不上；合同长这样，留着
	_ = id    // 单进程单 VM，id 备用
	os.Remove(f.Sock) // 清上次残留的 socket（有残留进程先 pkill -f firecracker-v1.17）
	f.proc = exec.Command(f.Bin, "--api-sock", f.Sock)
	if err := f.proc.Start(); err != nil {
		return fmt.Errorf("起 firecracker 失败: %v", err)
	}
	if err := f.waitSock(5 * time.Second); err != nil {
		return err
	}
	// 配 boot source / rootfs / 网络 / 机器：跟 fc-bench.sh 的四步一样
	if err := f.api("PUT", "/boot-source", fmt.Sprintf(
		`{"kernel_image_path":%q,"boot_args":"console=ttyS0 reboot=k panic=1 pci=off"}`, f.Kernel)); err != nil {
		return err
	}
	if err := f.api("PUT", "/drives/rootfs", fmt.Sprintf(
		`{"drive_id":"rootfs","path_on_host":%q,"is_root_device":true,"is_read_only":false}`, f.Rootfs)); err != nil {
		return err
	}
	// guest_mac 跟 fc-bench.sh 里的保持一致，不一样改这里
	if err := f.api("PUT", "/network-interfaces/eth0", fmt.Sprintf(
		`{"iface_id":"eth0","guest_mac":"AA:FC:00:00:00:01","host_dev_name":%q}`, f.Tap)); err != nil {
		return err
	}
	return f.api("PUT", "/machine-config", `{"vcpu_count":2,"mem_size_mib":512}`)
}

func (f *FirecrackerRuntime) Start(id string) error {
	_ = id
	if err := f.api("PUT", "/actions", `{"action_type":"InstanceStart"}`); err != nil {
		return err
	}
	// 轮询 ssh 直到就绪：跟 fc-bench.sh 的 until 循环一样，60 秒超时
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		err := exec.Command("ssh", "-i", f.Key, "-o", "BatchMode=yes",
			"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=2",
			"root@"+f.IP, "true").Run()
		if err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ssh %s 60s 未就绪", f.IP)
}

func (f *FirecrackerRuntime) Stop(id string) error {
	_ = id
	if f.proc != nil && f.proc.Process != nil {
		return f.proc.Process.Kill()
	}
	return nil
}

func (f *FirecrackerRuntime) Destroy(id string) error {
	_ = f.Stop(id)
	os.Remove(f.Sock)
	return nil
}
