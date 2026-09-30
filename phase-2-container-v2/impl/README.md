# 自制容器 v2.0 实现（Go）

对应路线：`docs/学习路线.md` Phase 2（Session 2-1 ~ 2-5）。

## 要实现的三层隔离

1. **cgroup v2**：建子 cgroup，写 `memory.max` / `cpu.max`，把容器 PID 写入 `cgroup.procs`
2. **seccomp**：`unix.Prctl`（`golang.org/x/sys/unix`）加载 profile，默认禁 `mount`、`reboot` 等
3. **网络**：建 netns + veth pair + NAT（`iptables MASQUERADE`），或复用 Session 2-5 的 shell 脚本

## 运行

```bash
cd phase-2-container-v2/impl
go run . run /bin/sh
```

## 依赖

- root 权限（VM 里 `sudo`）
- `golang.org/x/sys/unix`
