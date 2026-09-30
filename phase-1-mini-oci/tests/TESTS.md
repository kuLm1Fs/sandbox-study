# Phase 1 测试用例

在 Linux VM 或 Mac 上执行，当前目录为 `phase-1-mini-oci/`。以下用例在 mini-oci 实现完成后执行。T1-1/T1-2 可在 Mac 或 VM 执行；T1-3 的 `run` 需要 runc，必须在 Linux VM 执行；T1-4/T1-5 在 VM 执行。每条用例：操作 → 预期结果，通过才算实验做对。

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T1-1 | `go run ./impl pull busybox:latest` | 生成 `work/output/images/busybox/`，含 `index.json`、`oci-layout`、`blobs/sha256/`，且各文件 digest 与 manifest 里声明的一致 |
| T1-2 | `go run ./impl unpack busybox:latest` | 生成 `work/output/bundle/`，含 `rootfs/`（busybox 文件齐全）+ `config.json` |
| T1-3 | `go build -o work/mini-oci ./impl && sudo work/mini-oci run busybox:latest` | 容器启动并正常退出（exit 0）；退出后 `sudo runc list` 无残留（实现需 `runc delete` 清理） |
| T1-4 | `cd work/bundle`，`sudo runc spec` 生成 `config.json`，改 `process.args` 为 `["/hello", "wait"]`，`sudo runc run -d t1` | `sudo runc list` 看到 t1 为 running；`sudo runc kill t1 KILL && sudo runc delete t1` 后消失 |
| T1-5 | `sudo ctr image pull docker.io/library/busybox:latest`；另开终端 `sudo ctr run -t --rm docker.io/library/busybox:latest t1 /bin/sh`，再执行 `sudo ctr task exec -t --exec-id e1 t1 ls` | 容器跑起来，exec 能执行 `ls` 并看到输出 |
| T1-6 | 口头：`runc create` vs `runc run` 的区别 | 答出：create 只创建容器进程不启动，run = create + start |

**路径规矩**：所有实验产物都在 `work/` 下（git 忽略），`impl/` 里只有 Go 源码。`git status` 应看不到 `work/`。

**备注**：T1-6 答不上 → 回看 Session 1-3；T1-1/T1-2 失败 → 回看 Session 1-2。
