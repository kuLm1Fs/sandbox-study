// bench.go：三方案启动耗时对比（runc / runsc / firecracker）。
// 用法：sudo go run bench.go <firecracker_ms>
//   firecracker_ms：按讲义第 3 步手动测 5 次取中位数后填入。
// runc/runsc 部分自动跑；firecracker 部分手动计时（API + 等 ssh 就绪不好自动化）。
//
// 内存开销测的是"活着的容器"的 MemAvailable 差值：程序自动从 /tmp/bench/bundle
// 复制出一份 bundle-mem，把 config.json 的 args 改成 sleep，起 detached 容器
// 测完就删。不需要手动准备第二个 bundle。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

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
func timeCmd(dir, name string, args ...string) float64 {
	t := time.Now()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
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

// prepMemBundle 从 base 复制一份 bundle，把 config.json 的 process.args
// 改成 sleep，让内存测试有个"活着"的容器可测。返回新 bundle 的路径。
func prepMemBundle(base, dst string) string {
	os.RemoveAll(dst)
	if out, err := exec.Command("cp", "-r", base, dst).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "复制 bundle 失败: %v\n%s\n", err, out)
		os.Exit(1)
	}
	cfgPath := filepath.Join(dst, "config.json")
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读 config.json 失败: %v\n", err)
		os.Exit(1)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "解析 config.json 失败: %v\n", err)
		os.Exit(1)
	}
	proc, ok := cfg["process"].(map[string]any)
	if !ok {
		fmt.Fprintf(os.Stderr, "config.json 里没有 process 段\n")
		os.Exit(1)
	}
	proc["args"] = []string{"/bin/sh", "-c", "sleep 1000000"}
	out, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(cfgPath, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写 config.json 失败: %v\n", err)
		os.Exit(1)
	}
	return dst
}

func main() {
	const bundle = "/tmp/bench/bundle"
	fcMs := 0.0
	if len(os.Args) > 1 {
		fcMs, _ = strconv.ParseFloat(os.Args[1], 64)
	}

	// runc：前台 run 一个 exit 0 的容器，测 create+start+exit 全程，5 次取中位数。
	// 测完 delete 掉 stopped 的容器，保持环境干净。
	runcTs := []float64{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("bench-%d", i)
		runcTs = append(runcTs, timeCmd(bundle, "runc", "run", id))
		quiet("runc", "delete", id)
	}

	// runsc：经 ctr 跑 /bin/true，5 次取中位数（先 pull 好镜像）。
	runscTs := []float64{}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("gbench-%d", i)
		runscTs = append(runscTs, timeCmd("", "ctr", "run", "--rm",
			"--runtime", "io.containerd.runsc.v1",
			"docker.io/library/busybox:latest", id, "/bin/true"))
	}

	// 内存开销：起一个 detached 的 sleep 容器（活着），看 MemAvailable 差值；
	// 测完删掉。注意：不能拿 exit 0 的容器测——它退出后内存就被回收了，差值只是噪声。
	memBundle := prepMemBundle(bundle, "/tmp/bench/bundle-mem")
	m0 := memAvailMB()
	timeCmd("", "runc", "run", "-d", "--bundle", memBundle, "bench-mem")
	m1 := memAvailMB()
	quiet("runc", "delete", "--force", "bench-mem")

	fmt.Printf(`## 启动耗时（5 次中位数）

| 方案 | 中位数 |
|---|---|
| runc | %.0f ms |
| runsc (gVisor) | %.0f ms |
| firecracker | %.0f ms（手动填入） |

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
`, median(runcTs), median(runscTs), fcMs, m0-m1)
}
