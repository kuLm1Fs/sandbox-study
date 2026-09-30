# Phase 1 项目：mini-oci（Go）

**目标**：写一个 Go 小工具，打通「拉镜像 → 解包 → 生成 OCI bundle → runc 运行」全链路。

## 目录结构

- `impl/`：Go 实现（`go.mod` + 源码），`pull` / `unpack` / `run` 三个子命令
- `tests/`：测试用例（`TESTS.md`），每个用例含操作步骤和预期结果

## 功能

1. `pull`：从 registry 拉 manifest + layers（调 distribution-spec HTTP API）
2. `unpack`：解包成 rootfs + 生成 OCI bundle（`config.json`）
3. `run`：调 `runc run` 把它跑起来

## 验收

`go run ./impl run busybox:latest` 能跑起来。

## 进度

- [ ] Session 1-1 ~ 1-5（见 docs/学习路线.md）
- [ ] mini-oci 可用
