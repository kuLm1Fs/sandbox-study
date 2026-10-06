package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

var blockedSyscalls = []uint32{
	unix.SYS_MOUNT,
	unix.SYS_UMOUNT2,
	unix.SYS_PIVOT_ROOT,
	unix.SYS_REBOOT,
	unix.SYS_KEXEC_LOAD,
	unix.SYS_ACCT,
}

// installSeccomp 装一个 BPF 过滤器：命中黑名单就返回 EPERM，其余放行
//
// 它是“纵深防御“的第二层：即使你以 root 跑容器（有各种 capability）
// 这些系统调用也会被 ** 直接拒绝** --- 不靠权限体系，而是靠内核在这个进程上挂的钩子
func installSeccomp() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS: %w", err)
	}

	// BPF 程序就是一组指令，内核从第一条开始顺序执行。
	// 我们要判断的是 struct seccomp_data 里的 nr 字段（系统调用号），它在偏移 0。
	filter := make([]unix.SockFilter, 0, len(blockedSyscalls)*2+2)

	// 指令 1: 把 seccomp_data.nr 载入累加器 A
	filter = append(filter, unix.SockFilter{
		Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS,
		K:    0, // offsetof(seccomp_data, nr) == 0
	})
	for _, nr := range blockedSyscalls {
		filter = append(filter,
			unix.SockFilter{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, Jt: 0, Jf: 1, K: nr},
			unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
		)
	}
	filter = append(filter, unix.SockFilter{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW})

	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&prog)), 0, 0); err != nil {
		return fmt.Errorf("PR_SET_SECCOMP: %w", err)
	}
	return nil
}
