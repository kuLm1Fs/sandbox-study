package main

// OCI runtime-spec 的**最小可用子集**。
//
// 完整规范在 runtime-spec/config.md；这里只声明"能让 runc 跑起来"的部分。
// 对比 Session 1-3 里 `runc spec` 生成的那份：它还带 capabilities、rlimits、
// maskedPaths、readonlyPaths —— 那些是**加固项**，不是必需项（留作练习）。
type specUser struct {
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

type specProcess struct {
	Terminal bool     `json:"terminal"`
	User     specUser `json:"user"`
	Args     []string `json:"args"`
	Env      []string `json:"env"`
	Cwd      string   `json:"cwd"`
}

type specRoot struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly"`
}

type specMount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Source      string   `json:"source"`
	Options     []string `json:"options,omitempty"`
}

type specNamespace struct {
	Type string `json:"type"`
}

type specLinux struct {
	Namespaces []specNamespace `json:"namespaces"`
}

type runtimeSpec struct {
	OCIVersion string      `json:"ociVersion"`
	Process    specProcess `json:"process"`
	Root       specRoot    `json:"root"`
	Hostname   string      `json:"hostname"`
	Mounts     []specMount `json:"mounts"`
	Linux      specLinux   `json:"linux"`
}

// makeRuntimeSpec 把 image config 翻译成 runtime spec。
//
// 这就是 unpack 的"翻译"职责 —— 两个 spec 说的不是一回事：
//
//	image config（镜像作者说）: 默认跑什么命令、环境变量是什么
//	runtime spec（运行时要知道）: 起进程时 argv 是什么、挂哪些文件系统、怎么隔离
func makeRuntimeSpec(cfg imageConfig) runtimeSpec {
	// Docker 的合并规则：Entrypoint 在前，Cmd 是它的默认参数
	args := append([]string{}, cfg.Config.Entrypoint...)
	args = append(args, cfg.Config.Cmd...)
	if len(args) == 0 {
		args = []string{"/bin/sh"}
	}

	env := cfg.Config.Env
	if len(env) == 0 {
		env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	}

	return runtimeSpec{
		OCIVersion: "1.0.2",
		Process: specProcess{
			Terminal: false,
			User:     specUser{UID: 0, GID: 0},
			Args:     args,
			Env:      env,
			Cwd:      "/",
		},
		Root:     specRoot{Path: "rootfs", Readonly: false},
		Hostname: "mini-oci",
		// 这些挂载点 runc 会在容器里现挂：没有 /proc 和 /dev，容器基本没法用
		Mounts: []specMount{
			{Destination: "/proc", Type: "proc", Source: "proc"},
			{Destination: "/dev", Type: "tmpfs", Source: "tmpfs",
				Options: []string{"nosuid", "strictatime", "mode=755", "size=65536k"}},
			{Destination: "/dev/pts", Type: "devpts", Source: "devpts",
				Options: []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"}},
			{Destination: "/dev/shm", Type: "tmpfs", Source: "shm",
				Options: []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"}},
			{Destination: "/dev/mqueue", Type: "mqueue", Source: "mqueue",
				Options: []string{"nosuid", "noexec", "nodev"}},
			{Destination: "/sys", Type: "sysfs", Source: "sysfs",
				Options: []string{"nosuid", "noexec", "nodev", "ro"}},
			{Destination: "/sys/fs/cgroup", Type: "cgroup", Source: "cgroup",
				Options: []string{"nosuid", "noexec", "nodev", "relatime", "ro"}},
		},
		Linux: specLinux{
			Namespaces: []specNamespace{
				{Type: "pid"},     // 自己的进程号空间（容器里 PID 1 是第一个进程）
				{Type: "network"}, // 自己的网络栈（只有 lo）
				{Type: "ipc"},     // 自己的 IPC
				{Type: "uts"},     // 自己的 hostname
				{Type: "mount"},   // 自己的挂载表（pivot_root 的前提）
				{Type: "cgroup"},  // 自己的 cgroup 视图
			},
		},
	}
}
