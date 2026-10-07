// bench.go：三方案启动耗时对比（runc / runsc / firecracker），经 docker 测。
// 用法：go run bench.go [firecracker_ms]
//   firecracker_ms：按讲义第 3 步手动测 5 次取中位数后填入（毫秒）。
// runc/runsc 部分自动跑（docker --runtime=...，3-3 已验证）；
// firecracker 部分手动计时（API + 等 ssh 就绪不好自动化）。
//
// 说明：经 docker 测的是"容器启动"耗时（含 docker 固定开销），
// 三组用同一 harness，docker 开销对两组一样，对比倍率可信；
// 绝对值比裸 runc 大，读数时注意。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const image = "docker.m.daocloud.io/library/busybox:latest"

func median(ds []float64) float64 {
	sort.Float64s(ds)
	return ds[len(ds)/2]
}

func memAvailMB() float64 {
	b, _ := os.ReadFile("/proc/meminfo")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "MemAvailable:") {
			f := strings.Fields(l)
			kb, _ := strconv.ParseFloat(f[1], 64)
			return kb / 1024
		}
	}
	return 0
}

// timeCmd 跑外部命令，返回耗时毫秒；失败直接退出（计时就别吞错了）。
func timeCmd(name string, args ...string) float64 {
	t := time.Now()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "命令失败 %s %v: %v\n%s\n", name, args, err, out)
		os.Exit(1)
	}
	return float64(time.Since(t).Milliseconds())
}

// quiet 跑命令，报错也不退出（只用于清理，失败了无所谓）。
func quiet(name string, args ...string) {
	_ = exec.Command(name, args...).Run()
}

func main() {
	fcMs := 0.0
	if len(os.Args) > 1 {
		fcMs, _ = strconv.ParseFloat(os.Args[1], 64)
	}

	// runc / runsc：经 docker 各跑 5 次 /bin/true，测容器启动全程，取中位数。
	runcTs := []float64{}
	for i := 0; i < 5; i++ {
		runcTs = append(runcTs, timeCmd("docker", "run", "--rm", "--runtime=runc", image, "/bin/true"))
	}
	runscTs := []float64{}
	for i := 0; i < 5; i++ {
		runscTs = append(runscTs, timeCmd("docker", "run", "--rm", "--runtime=runsc", image, "/bin/true"))
	}

	// 内存开销：起一个活着的 sleep 容器，看 MemAvailable 差值；测完删掉。
	// 注意：不能拿已退出的容器测——它退出后内存就被回收了，差值只是噪声。
	m0 := memAvailMB()
	timeCmd("docker", "run", "-d", "--runtime=runc", "--name", "bench-mem", image, "sleep", "1000000")
	time.Sleep(2 * time.Second) // 等容器进程稳定
	m1 := memAvailMB()
	quiet("docker", "rm", "-f", "bench-mem")

	fcRow := "（按讲义第 3 步手动测 5 次取中位数后填入）"
	if fcMs > 0 {
		fcRow = fmt.Sprintf("%.0f ms", fcMs)
	}
	fmt.Printf(`## 启动耗时（5 次中位数，经 docker 测容器启动）

| 方案 | 中位数 |
|---|---|
| runc | %.0f ms |
| runsc (gVisor) | %.0f ms |
| firecracker | %s |

## 内存开销（单实例，MemAvailable 差值）

| 方案 | 差值 |
|---|---|
| runc 起一个活着的容器 | %.1f MB |
| firecracker | ≈ guest 内存（512MB 配置 → 约 550MB，含 VMM 开销） |

## 对比表

| | 隔离边界 | 启动速度 | 内存开销 | 适用场景 |
|---|---|---|---|---|
| runc | 进程级（namespace+seccomp，共享内核） | 最快 | ≈0 | 普通容器 |
| gVisor | syscall 拦截（用户态内核） | 中 | 小 | 不可信代码、多租户容器 |
| firecracker | 硬件虚拟化（真内核） | 慢（百 ms 级） | 大（guest 内存） | 强隔离的 serverless/沙盒 |

## 结论（按你的实测数据填）

1. 启动速度：___ 最快（约 ___ms），适合 ___。
2. 隔离强度：___ 最强（___），代价是 ___。
3. Agent 沙盒选型：如果 ___ 选 ___，因为 ___。
`, median(runcTs), median(runscTs), fcRow, m0-m1)
}
