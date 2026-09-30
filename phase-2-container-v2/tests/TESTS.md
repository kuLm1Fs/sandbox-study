# Phase 2 测试用例

在 Linux VM 里执行（需要 root）。

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T2-1 | 建 cgroup 子目录，`memory.max=100M`，把当前 shell PID 写入 `cgroup.procs`，跑 `/tmp/membomb.go`（200MB） | 进程被 OOM killer 杀掉；宿主机其他进程不受影响 |
| T2-2 | `go run ./impl run /bin/sh` 起 v2.0 容器，在容器里跑内存炸弹 | 容器内进程被 kill，宿主机正常（cgroup 生效） |
| T2-3 | 在 v2.0 容器里执行 `mount` | 返回 `EPERM` / Operation not permitted（seccomp 生效） |
| T2-4 | `mount -t overlay` 实验：改 `merged/file` 后观察 | `upper/` 出现新文件，`lower/` 内容不变 |
| T2-5 | 按 Session 2-5 配好 netns + veth + NAT，`ip netns exec ns1 ping -c2 8.8.8.8` | 能 ping 通；宿主机 `ip addr` 看不到 ns1 的 IP（隔离） |

**备注**：T2-1 失败 → 重看 Session 2-1 的三个文件作用；T2-5 卡住 → 先降级到"两 netns 互 ping"。
