# sandbox-compare 实现（Go）

对应路线：`docs/学习路线.md` Phase 3（Session 3-1 ~ 3-5）。

## 要实现的程序

`bench.go`：对比 runc / runsc(gVisor) / Firecracker 三种方案

1. 启动耗时：各起 5 次，取中位数
2. 内存开销：启动后记录宿主机新增内存
3. 输出 markdown 表格：隔离边界 / 启动速度 / 内存开销 / 适用场景 + 3 条结论

## 运行

```bash
cd phase-3-sandbox-compare/impl
go run bench.go <firecracker中位数> > ../RESULTS.md
```

`firecracker中位数`：按讲义 Session 3-5 第 3 步手动测 5 次（`InstanceStart` → ssh 就绪）取中位数后填入。
runc/runsc 部分自动跑（经 `docker --runtime=...`，3-3 已验证；不用 sudo）。

## 依赖

- WSL2 / x86_64（`/dev/kvm` 可用，3-2 已验证）
- Docker（`runc`、`runsc` 两个 runtime 均已注册，3-3 已验证）+ Firecracker 二进制（x86_64）
- 镜像 `docker.m.daocloud.io/library/busybox:latest` 已 pull（Docker Hub 直连不通，走 daocloud）

## 实机改写说明（2026-10-07）

原讲义是裸 `runc` + `ctr` 路径。实机发现 WSL 的 containerd socket 与 `ctr`
默认的不一致，`ctr plugins ls` 看不到 runsc；改走 3-3 已验证的
`docker --runtime=runc|runsc`。同一 harness 下 docker 的固定开销对两组一样，
对比倍率可信；绝对值比裸 runc 大，读 RESULTS.md 时注意。
