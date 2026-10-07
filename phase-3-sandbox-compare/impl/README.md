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
sudo go run bench.go <firecracker中位数> > ../RESULTS.md
```

`firecracker中位数`：按讲义 Session 3-5 第 3 步手动测 5 次（`InstanceStart` → ssh 就绪）取中位数后填入。
runc/runsc 部分自动跑；bench.go 会自己从 `/tmp/bench/bundle` 复制出一份
`bundle-mem`（args 改成 sleep）来测"活着的容器"的内存差值，不用手动准备第二个 bundle。

## 依赖

- WSL2 / x86_64（`/dev/kvm` 可用，3-2 已验证）
- `runc`、`runsc`（经 containerd `ctr` 调用）、Firecracker 二进制（x86_64）
- 开工前先确认 `ctr` 的 `io.containerd.runsc.v1` runtime 可用；若不可用，改用
  3-3 已验证的 `docker --runtime=runsc` 路径（见讲义）
