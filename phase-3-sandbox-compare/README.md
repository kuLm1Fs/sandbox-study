# Phase 3 项目：sandbox-compare

**目标**：对比 runc / gVisor(runsc) / Firecracker，输出一张面试能讲 10 分钟的对比表。

## 内容

- `bench.py`：测启动耗时（各 5 次取中位数）+ 内存开销
- `RESULTS.md`：对比表（隔离边界 / 启动速度 / 内存开销 / 适用场景）+ 3 条结论

## 验收

- Firecracker microVM、gVisor、Kata 各跑通一次
- RESULTS.md 有对比表和结论

## 进度

- [ ] Session 3-1 ~ 3-5（见 docs/学习路线.md）
- [ ] bench.py + RESULTS.md
