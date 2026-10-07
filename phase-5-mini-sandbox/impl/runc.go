package main

import "os/exec"

// RuncRuntime 用 docker CLI 实现 Runtime 合同。
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
