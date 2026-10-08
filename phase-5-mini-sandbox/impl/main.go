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
	var rt Runtime = NewFirecrackerRuntime()

	// Snapshotter 是可选能力：类型断言检查这个后端会不会快照。
	// 换成 &RuncRuntime{} 跑，这行就进不去——不是所有 Runtime 都会快照。
	s, ok := rt.(Snapshotter)
	if !ok {
		panic("当前后端不支持快照")
	}

	id := "w4demo"
	snap, mem := "/tmp/w4.snap", "/tmp/w4.mem"

	// 1. 起 VM，在客户机内存里留个记号：一个后台 sleep 进程（只活在内存里）。
	must(rt.Create(id, "docker.m.daocloud.io/library/busybox:latest"))
	must(rt.Start(id))
	// 整个脚本是一个参数传给 sh -c（Exec 会加引号保证它不被拆开）。
	// W4 实机教训：Exec 以前没加引号，sh -c 只吃到 "nohup" 一个词，后台起的是个空进程。
	out, err := rt.Exec(id, "sh", "-c", "nohup sleep 300 >/dev/null 2>&1 </dev/null & echo $!")
	must(err)
	fmt.Println("guest bg pid:", strings.TrimSpace(out))

	// 2. 拍快照（Full：内存+设备状态全存文件），然后销毁原 VM。
	must(s.Snapshot(id, snap, mem))
	fmt.Println("snapshot done")
	must(rt.Stop(id))
	must(rt.Destroy(id))

	// 3. 从快照唤醒一个新 VM。
	must(s.Restore(id, snap, mem))
	fmt.Println("restored")

	// 4. 验收：那个只活在内存里的 sleep 进程还活着吗？
	// 活着 = 内存状态真的被恢复了（它没写过磁盘，作不了弊）。
	out, err = rt.Exec(id, "pgrep", "-f", "sleep 300")
	pid := strings.TrimSpace(out)
	if err != nil || pid == "" {
		panic("SNAPSHOT FAIL: 后台进程没活下来（内存状态没恢复）")
	}
	fmt.Println("surviving pid:", pid)
	fmt.Println("SNAPSHOT PASS")

	must(rt.Stop(id))
	must(rt.Destroy(id))
	fmt.Println("sandbox destroyed")
}
