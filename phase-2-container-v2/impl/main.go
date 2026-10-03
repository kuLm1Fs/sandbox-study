package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

func main() {

	// 判断参数量，如果小于 2 直接 error 退出
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "At least 2 Arguments\n")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		run()
	case "child":
		child()
	default:
		panic("help")
	}
}

func run() {
	fmt.Printf("Running %v \n", os.Args[2:])

	cmd := exec.Command("proc/self/exe", append([]string{"child"}, os.Args[2:]...)...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:   syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNS,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	must(cmd.Run())
}

func child() {
	fmt.Printf("Running %v in %v child process", os.Args[2:], os.Getegid())

	cmd := exec.Command(os.Args[2], os.Args[3:]...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(syscall.Sethostname([]byte("container")))
	must(syscall.Chroot("/home/liz/ubuntufs"))
	must(os.Chdir("/"))
	must(syscall.Mount("proc", "proc", "proc", 0, ""))
	must(syscall.Mount("thing", "mytemp", "tepfs", 0, ""))

	cg()

	must(cmd.Run())

	must(syscall.Unmount("proc", 0))
	must(syscall.Unmount("thing", 0))
}

func cg() {
	cgroups := "/sys/fs/cgroup/"
	kul_gp := filepath.Join(cgroups, "kul")
	must(os.Mkdir(kul_gp, 0755))

	if _, err := os.Stat(filepath.Join(kul_gp, "pids.max")); err != nil {
		panic(fmt.Errorf("pids controller 未启用，找不到 %s：%w", filepath.Join(kul_gp, "pids.max"), err))
	}

	must(os.WriteFile(filepath.Join(kul_gp, "pids.max"), []byte("20"), 0644))
	// removes the new cgroup in place after the container exits
	must(os.WriteFile(filepath.Join(kul_gp, "cgroup.procs"),
		[]byte(strconv.Itoa(os.Getpid())),
		0644,
	))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
