package main

import (
	"fmt"
	"strings"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	// 关键行：*FirecrackerRuntime 赋值给 Runtime 接口变量。
	// 从这里开始，main 只知道 rt 会 Create/Start/Stop/Destroy/Exec，
	// 不知道底下是 docker，还是 firecracker。
	var rt Runtime = NewFirecrackerRuntime()
	// 验收期望：firecracker 的客户机内核是 6.18.x；
	// 换 runc 后端时改成宿主内核关键字（比如 "microsoft-standard"）。
	expect := "6.18"

	id := "w3demo"
	must(rt.Create(id, "docker.m.daocloud.io/library/busybox:latest"))
	must(rt.Start(id))

	// W3 验收：在沙盒里跑 uname -r，检查输出是否符合期望。
	// runc 版 Exec = docker exec，firecracker 版 Exec = ssh，
	// 但 main 里看到的都是同一个 rt.Exec。
	out, err := rt.Exec(id, "uname", "-r")
	must(err)
	kernel := strings.TrimSpace(out)
	fmt.Println("sandbox kernel:", kernel)
	if !strings.Contains(kernel, expect) {
		panic(fmt.Sprintf("VERIFY FAIL: 期望包含 %q，实际 %q", expect, kernel))
	}
	fmt.Println("VERIFY PASS")

	must(rt.Stop(id))
	must(rt.Destroy(id))
	fmt.Println("sandbox destroyed")
}
