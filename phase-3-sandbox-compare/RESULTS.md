# RESULTS.md — Session 3-5 三方案对比评测

实机环境：WSL2 / x86_64，Docker 29.8.2，runsc release-20260928.0，
Firecracker v1.17.0，Go go1.26.0。日期：2026-10-07。

## 启动耗时（5 次中位数）

| 方案 | 中位数 | 测法 |
|---|---|---|
| runc | 466 ms | `docker run --rm --runtime=runc` 跑 `/bin/true` |
| runsc (gVisor) | 451 ms | `docker run --rm --runtime=runsc` 跑 `/bin/true` |
| firecracker | 1391 ms | `InstanceStart` → `ssh root@172.16.0.2 true` 就绪，5 次原始值：1449 / 1346 / 1391 / 1384 / 1395 |

说明：容器两组经同一 Docker harness 测量，Docker 的固定开销对两组相同，
对比的相对关系可信；绝对值比裸 `runc` 大。Firecracker 经 `fc-bench.sh`
计时（API 调用 + guest 启动 + ssh 轮询三段，不好自动化，手动 5 次取中位数更诚实）。

## 内存开销（单实例）

| 方案 | 结果 |
|---|---|
| runc（一个活着的容器） | MemAvailable 差值 -9.5 MB —— 出现负数，说明该测法噪声太大，本次不下结论 |
| firecracker | 未实测；理论估算 ≈ guest 内存（512 MiB 配置 → 约 550 MB，含 VMM 开销） |

## 对比表

| | 隔离边界 | 启动速度 | 内存开销 | 适用场景 |
|---|---|---|---|---|
| runc | 进程级（namespace + seccomp，共享宿主内核） | 最快（466 ms） | ≈0 | 普通容器 |
| gVisor | syscall 拦截（用户态内核） | 与 runc 同一水平（451 ms） | 小 | 不可信代码、多租户容器 |
| firecracker | 硬件虚拟化（独立 guest 内核） | 慢（1391 ms，约 3x） | 大（guest 内存量级） | 强隔离的 serverless / 沙盒 |

## 结论

1. **runc 与 runsc 启动耗时基本同一水平**：451 ms vs 466 ms，15 ms
   的差距在噪声范围内，不能得出谁更快的结论。gVisor 用户态内核
   （Sentry/Gofer）的启动开销被 Docker harness 的固定开销掩盖了；
   要对比运行时本身的启动代价，需要去掉 Docker 层直接测。
2. **Firecracker microVM 冷启动约 1.4 s，是容器启动的 ~3 倍**：
   多出来的时间主要是 guest 内核启动 + userspace 初始化 + sshd 就绪。
   这是"真虚拟机"隔离的本质代价——硬件虚拟化、独立内核的强隔离边界，
   换来更慢的冷启动；对 serverless 场景意味着必须做快照或预热。
3. **内存开销本次没有可信数字**：`MemAvailable` 差值测出 -9.5 MB
   （负数），证明"起容器前后读 /proc/meminfo"这种粗测法噪声太大。
   内存对比需要 cgroup memory peak 这类精细工具，本次不下结论，留作后续。
