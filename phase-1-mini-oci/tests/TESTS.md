# Phase 1 测试用例

在 Linux VM 里执行。每条用例：操作 → 预期结果，通过才算实验做对。

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T1-1 | `go run ./impl pull busybox:latest` | 本地生成 OCI layout 目录，含 `index.json`、`oci-layout`、`blobs/sha256/`，且 digest 能对上 |
| T1-2 | `go run ./impl unpack <上一步的目录>` | 生成 `rootfs/`（busybox 文件齐全）+ bundle `config.json` |
| T1-3 | `go run ./impl run busybox:latest` | 容器启动并正常退出（exit 0），`runc list` 曾出现过该容器 |
| T1-4 | `sudo runc spec` 生成 bundle 后 `sudo runc run t1` | 容器运行；`runc list` 可见；`runc kill t1` 后消失 |
| T1-5 | `ctr image pull docker.io/library/busybox:latest` + `ctr run` | 容器跑起来，能 `ctr task exec` 进去执行 `ls` |
| T1-6 | 口头：`runc create` vs `runc run` 的区别 | 答出：create 只创建容器进程不启动，run = create + start |

**备注**：T1-6 答不上 → 回看 Session 1-3；T1-1/T1-2 失败 → 回看 Session 1-2。
