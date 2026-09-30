# mini-oci 实现（Go）

对应路线：`docs/学习路线.md` Phase 1（Session 1-1 ~ 1-5）。

## 要实现的子命令

- `pull <image>`：调 distribution-spec HTTP API，拉 manifest + layers，存为 OCI layout 目录
- `unpack <layout>`：解包 layers 成 rootfs，生成 OCI runtime bundle（`config.json`）
- `run <image>`：pull + unpack + 调 `runc run` 一条龙

## 运行

```bash
cd phase-1-mini-oci/impl
go run . run busybox:latest
```

## 依赖

- `runc`（Linux VM 里）
- 标准库为主：`net/http`、`archive/tar`、`crypto/sha256`、`encoding/json`
