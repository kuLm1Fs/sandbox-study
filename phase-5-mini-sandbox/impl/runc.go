package main

import "os/exec"

// RuncRuntime 用 docker CLI 驱动 runc，实现 Runtime 合同。
//
// 诚实声明：这里并没有直驱 runc。真实调用链是：
//
//	本程序 → docker CLI → dockerd → containerd → runc
//
// runc 在最底下真正干活，我们隔了两层——是"借 docker 之手调 runc"。
// 这么取舍是因为四个接口方法跟 docker CLI 一一对应；
// 直驱 runc 需要自备 bundle（config.json + rootfs），全是体力活
// （3-5 原计划就是这么死的），对理解 interface 零帮助。
//
// 注意：这里没有写"implements Runtime"——Go 里只要方法齐了，
// 编译器就认你实现了接口，这是 interface 最核心的一点。
// 下面 var _ Runtime 那行是编译期检查：方法缺一个就编不过。
type RuncRuntime struct{}

var _ Runtime = (*RuncRuntime)(nil)

func (r *RuncRuntime) Create(id, image string) error {
	return exec.Command("docker", "create", "--name", id, "--runtime=runc", image, "sleep", "3600").Run()
}

func (r *RuncRuntime) Start(id string) error {
	return exec.Command("docker", "start", id).Run()
}

func (r *RuncRuntime) Stop(id string) error {
	return exec.Command("docker", "stop", id).Run()
}

func (r *RuncRuntime) Destroy(id string) error {
	return exec.Command("docker", "rm", "-f", id).Run()
}
