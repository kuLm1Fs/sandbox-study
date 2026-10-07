package main

import "fmt"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	// 关键行：*RuncRuntime 赋值给 Runtime 接口变量。
	// 从这里开始，main 只知道 rt 会 Create/Start/Stop/Destroy，
	// 不知道底下是 docker，还是 W2 换成的 firecracker。
	var rt Runtime = &RuncRuntime{}

	id := "w1demo"
	must(rt.Create(id, "docker.m.daocloud.io/library/busybox:latest"))
	must(rt.Start(id))
	fmt.Println("sandbox running:", id)
	must(rt.Stop(id))
	must(rt.Destroy(id))
	fmt.Println("sandbox destroyed")
}
