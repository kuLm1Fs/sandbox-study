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
go run . > ../RESULTS.md
```

## 依赖

- Linux VM（`/dev/kvm` 存在，Lima 嵌套虚拟化已开）
- `runc`、`runsc`、Firecracker 二进制（aarch64）
