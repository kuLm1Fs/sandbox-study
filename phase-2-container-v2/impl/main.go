// mini-container：从零写的容器（Phase 2）。
//
//	run   → 在宿主机上执行（需要 root）：clone 出带 namespace 的子进程 + 建 cgroup
//	child → 在容器里执行（新 PID namespace 的 PID 1）：挂载 + pivot_root + exec
//
// 两个角色的交接方式：run 用 /proc/self/exe **重新执行自己**，参数里带 "child"。
// 因为 namespace 只能在"创建进程的那一刻"设置（它是 clone 的参数），
// 所以必须有一个"已经在容器 namespace 里、但还没换根"的中间状态来干准备工作。
// runc 里这个角色的名字叫 `runc init`。
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

var (
	memLimit  = flag.String("memory", "100M", "容器内存上限（cgroup memory.max, 如 100M、1G、max）")
	pidsLimit = flag.String("pids", "20", "容器进程数上限（cgroup pids.max")
)

// rootfs 从哪来：phase-1-mini-oci 的工具解包出来的那棵树。
// ⚠️ 相对路径是相对**当前工作目录**的，所以要在 phase-2-container-v2/ 下执行。
const rootfsDir = "../phase-1-mini-oci/work/output/bundle/rootfs"

// cgroup 路径。v2 是"一个 cgroup 一个目录"，层级由目录树表达。
const cgroupDir = "/sys/fs/cgroup/mini-container"

func usage() {
	fmt.Fprint(os.Stderr, `mini-container —— 从零写的容器

用法:
  sudo mini-container run <命令> [参数…]

例:
  sudo mini-container run /bin/sh
`)
}

func main() {

	flag.Parse()
	// child 分支：由 run() 通过 /proc/self/exe 重新执行进入（argv[1] == "child"）
	if len(os.Args) > 1 && os.Args[1] == "child" {
		must(child())
		return
	}

	args := flag.Args()
	if len(args) < 2 || args[0] != "run" {
		usage()
		os.Exit(2)
	}
	must(run(args[1], args[2:]))
}

// run 跑在**宿主机**上（root），干两件事：
//
//	① clone 出带新 namespace 的子进程
//	② 建 cgroup 并把子进程放进去
//
// 为什么 cgroup 由父进程管：cgroup **不属于任何 namespace**，它是宿主机的资源视图。
// 父进程没有新建 PID namespace，用它看到的 PID（cmd.Process.Pid）最不容易出歧义。
// docker/containerd 也是这个分工：由 shim/父进程建 cgroup，再把容器进程放进去。
func run(name string, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("需要 root（要建 namespace、挂载、写 cgroup）")
	}
	rootfs, err := filepath.Abs(rootfsDir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(rootfs, "bin")); err != nil {
		return fmt.Errorf("rootfs 不可用(%s)：先在 phase-1-mini-oci 里跑 `go run ./impl unpack busybox:latest`", rootfs)
	}

	// ⚠️ 开头的 "/" 必须有：少了它就是相对路径，Go 会按 cwd 去找 "proc/self/exe"
	cmd := exec.Command("/proc/self/exe", append([]string{"child", rootfs, name}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | // 自己的进程号空间 → 容器里第一个进程是 PID 1
			syscall.CLONE_NEWUTS | // 自己的 hostname
			syscall.CLONE_NEWNS | // 自己的挂载表（pivot_root 的前提）
			syscall.CLONE_NEWIPC, // 自己的 IPC（不加就是和宿主机共用）
		// 注意：这里**没有** CLONE_NEWNET —— 网络不再由容器自己建，
		// 而是宿主机先把 netns 建好、接好 veth，容器再 setns 进去（见 network.go）
	}

	// 接线在容器启动**之前**做（父进程干这活最自然：只有它能碰宿主机的网络）
	if err := setupNetwork(); err != nil {
		return err
	}
	defer cleanupNetwork() // 容器退出后把网线和 NAT 规则撤掉

	if err := enableNAT(); err != nil {
		return err
	}

	if err := setupCgroup(); err != nil {
		return err
	}
	defer removeCgroup() // 容器退出后把空目录删掉

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := putInCgroup(cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	err = cmd.Wait()
	dumpCgroupStats()
	if err != nil {
		return fmt.Errorf("容器进程退出：%w", err)
	}

	removeCgroup()

	return nil
}

// setupCgroup 建 cgroup 并设上限 —— 这是"资源隔离"的第一层：
// 进程超了就被内核收拾（OOM kill / fork 失败），宿主机的其他进程不受影响。
//
// 注意执行时机：必须在 chroot/pivot_root **之前**由父进程做。
// 一旦换根，/sys/fs/cgroup 这个绝对路径就会在容器的新根里解析（而那里没有 /sys）。
func setupCgroup() error {
	if err := os.MkdirAll(cgroupDir, 0o755); err != nil {
		return fmt.Errorf("建 cgroup %s: %w", cgroupDir, err)
	}
	limits := map[string]string{
		"pids.max":   *pidsLimit,
		"memory.max": *memLimit,
	}
	for file, val := range limits {
		p := filepath.Join(cgroupDir, file)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("控制器不可用，找不到 %s：%w", p, err)
		}
		if err := os.WriteFile(p, []byte(val), 0o644); err != nil {
			return fmt.Errorf("写 %s = %s: %w", file, val, err)
		}
	}
	return nil
}

// putInCgroup 把容器进程放进 cgroup。
// 这里传的是**宿主机视角**的 PID（cmd.Process.Pid）。
// 容器里那个进程看到的自己是 1，但那是另一个 PID namespace 里的编号 ——
// 写 cgroup.procs 时内核按**写者所在的 PID namespace**解析，所以由父进程写才不会有歧义。
func putInCgroup(pid int) error {
	p := filepath.Join(cgroupDir, "cgroup.procs")
	if err := os.WriteFile(p, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("把 pid %d 放进 cgroup: %w", pid, err)
	}
	return nil
}

func removeCgroup() {
	// 组里还有进程时 rmdir 会失败；容器退出后应该是空的
	_ = os.Remove(cgroupDir)
}

// child 跑在**容器里**，是这个新 PID namespace 的 PID 1。
// 步骤顺序不能乱 —— 每一步都依赖前一步的结果。
func child() error {
	rootfs := os.Args[2]
	name := os.Args[3]
	args := os.Args[4:]
	runtime.LockOSThread()

	// 0) 先加入宿主机预先建好的 netns（必须在 pivot_root 之前：
	//    换根之后 /var/run/netns 就看不见了）
	if err := joinNetns(); err != nil {
		return err
	}

	// 1) 把根挂载设成"私有"。
	//    新 mount namespace 会**继承父级的传播类型**（通常是 shared），
	//    不设私有的话，容器里做的挂载可能传播回宿主机（在宿主机 mount 表里冒出来）。
	//    真正解决泄漏的是这一行，而不是 Unshareflags。
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("把 / 设为私有: %w", err)
	}

	// 1) 设 hostname（UTS namespace 已经独立，改了不影响宿主机）
	if err := syscall.Sethostname([]byte("mini-container")); err != nil {
		return fmt.Errorf("sethostname: %w", err)
	}

	// 2) 把 rootfs bind mount 到它自己。
	//    pivot_root 的硬性要求：new_root 必须是一个**挂载点**，不能只是普通目录。
	if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind mount rootfs: %w", err)
	}

	// 3) 重新挂 /proc。换了 PID namespace 之后必须重挂，
	//    否则容器里看到的还是**宿主机的进程列表**（ps 会列出宿主机所有进程）。
	procPath := filepath.Join(rootfs, "proc")
	if err := os.MkdirAll(procPath, 0o755); err != nil {
		return err
	}
	if err := syscall.Mount("proc", procPath, "proc", 0, ""); err != nil {
		return fmt.Errorf("mount /proc: %w", err)
	}

	// 4) pivot_root：new_root=rootfs，old_root 必须在 new_root **之内**（接口要求）。
	if err := os.MkdirAll(filepath.Join(rootfs, ".oldroot"), 0o700); err != nil {
		return err
	}
	if err := syscall.PivotRoot(rootfs, filepath.Join(rootfs, ".oldroot")); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	// 卸掉旧根 —— 这是 pivot_root 相对 chroot 的核心优势：
	// chroot 只改"眼里的根"，旧根还挂在，理论上能被绕过去；
	// 卸掉之后旧根从挂载表里消失，无路可逃。runc 用的就是这个。
	if err := syscall.Unmount("/.oldroot", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("卸掉旧根: %w", err)
	}
	_ = os.Remove("/.oldroot") // 删掉这个空目录；失败无所谓

	// 5) execve：把自己整个换成目标程序。执行成功后，"准备阶段"的进程就不存在了，
	//    目标程序直接接手 PID 1。
	if err := installSeccomp(); err != nil {
		return err
	}
	env := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	return syscall.Exec(name, append([]string{name}, args...), env)
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "mini-container: %v\n", err)
		os.Exit(1)
	}
}

// dumpCgroupStats 打印 cgroup 记的账。容器被 OOMkiller 干掉时
// 这是最直接的证据（否则只能看到 “exit status 137”， 不知道为什么）。
func dumpCgroupStats() {
	max, _ := os.ReadFile(filepath.Join(cgroupDir, "memory.max"))
	peak, _ := os.ReadFile(filepath.Join(cgroupDir, "memory.peak"))
	events, _ := os.ReadFile(filepath.Join(cgroupDir, "memory.events"))
	fmt.Printf("--- cgroup 现场（memory.max=%s memory.peak= %s) --- \n%s",
		trimNL(max), trimNL(peak), events)
}

func trimNL(b []byte) string {
	return string(bytes.TrimSpace(b))
}
