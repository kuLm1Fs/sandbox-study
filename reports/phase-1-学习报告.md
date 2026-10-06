# Phase 1 学习报告

> 阶段：容器运行时进阶 ｜ 周期：2026-10-01 ~ 2026-10-02 ｜ 状态：✅ 完成

## 交付物

**mini-oci**（Go，988 行，`phase-1-mini-oci/impl/`）：

| 子命令 | 做的事 | 关键实现 |
|---|---|---|
| `pull` | 顺引用链从 registry 拉镜像 | token 401 自动换发重试；307 重定向跟随 CDN；多架构 index 按 arm64 挑选；本地 index 自己造；blob 内容寻址缓存 |
| `unpack` | 解压 layer 叠成 rootfs | gzip 解压 + tar 遍历；`TeeReader` 流式算 diff_id；whiteout/opaque；`prepareTarget` 处理层间文件类型更换；`safeJoin` 防路径越界 |
| `run` | 拼 bundle 调 runc | image config → OCI runtime bundle；`runc run` + 退出后 `runc delete` 清理 |

手工参照物：`tools/handpull.sh` / `tools/mkmultilayer.sh`。

## 测试状态（`tests/TESTS.md`）

| 编号 | 结果 |
|---|---|
| T1-1 `pull busybox:latest` | ✅ 通过（2026-10-02） |
| T1-2 `unpack busybox:latest` | ✅ 通过（2026-10-02） |
| T1-3 `run busybox:latest` | ✅ 通过（2026-10-02） |
| T1-4 / T1-5 / T1-6 | ⏳ 待补（对应 session 当时未完成） |

## 知识点掌握（`知识点.md` K1-1~K1-10）

- 主线 K1-1~K1-7：✅（入口自测 5 题全过；四个结构体 + 引用链已能脱口而出）
- 支线 K1-8~K1-10：✅（在写 mini-oci 时覆盖：`writeBlob` 原子发布、`prepareTarget`、config→bundle）

## 错题统计

[错题本](../phase-1-mini-oci/mistakes/错题本.md) 共 12 条，全部来自实机。按类型：静默错误类 3 条（最贵）、环境/路径类 3 条、registry 协议类 3 条、runc 行为类 2 条、Go 编译类 1 条。

最贵的一条：#6 `os.ReadFile` 丢 error 打出空 layer——OCI 结构完全合法，直到 `docker run` 才炸。教训：静默错误比报错贵一个数量级。

## 方法复盘

- 有效：session 制（25–40 分钟一节）+ "一条命令 + 预期输出"——Phase 1 全程按此执行，无囤积。
- 有效：真实镜像测试——`busybox` 单层镜像测不出 unpack 的层间覆盖 bug，`python:3.12-alpine` 4 层镜像一次暴露（`638aa50`）。
- 待改进：T1-4~T1-6 当时没条件验证，拖成了尾巴——以后"测试写完就跑，不过夜"。

## 下一步

→ Phase 2（Linux 隔离与安全原语）。Phase 1 的 whiteout 知识在 2-4（overlayfs）会回来看；`prepareTarget` 的"层怎么叠"是 2-4 的预习。
