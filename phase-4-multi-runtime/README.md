# Phase 4 项目：multi-runtime 集群

**目标**：单节点 k3s + 两个 RuntimeClass（runc 默认 + runsc），验证多 runtime 共存。
2026-10-07 在 WSL2 k3s 上实机跑通。

## 目录结构

- `impl/`：RuntimeClass + demo pod 的 YAML 清单
- `tests/`：测试用例（`TESTS.md`，T4-1 ~ T4-4 实机通过）
- `notes/`：Session 4-1 ~ 4-6 精华问答（`学习笔记.md`）

## 实机记录：怎么接入一个新 runtime（以 runsc 为例，kata 同理）

两处，缺一不可：

1. **后厨**（containerd）：`config.toml` 存成 `config.toml.tmpl` 模板，
   追加 runtime 段后 `systemctl restart k3s`。注意两点：
   - 直接改 `config.toml` 会被 k3s 重启重写，必须改模板；
   - `config-v3.toml.d/` drop-in 在这版 k3s 上没被加载
     （实测 `grep -c kata config.toml` = 0），别用。

   ```toml
   [plugins."io.containerd.cri.v1.runtime".containerd.runtimes.runsc]
     runtime_type = "io.containerd.runsc.v1"
   ```

   以 kata-fc 为例：同理加一段 `runtimes.kata-fc`，
   `runtime_type = "io.containerd.kata-fc.v2"`，
   `runtime_path = "/opt/kata/bin/containerd-shim-kata-v2"`——
   chart 留下的 `config-v3.toml.d/kata-deploy.toml` 里有现成写法，照抄一段即可。

2. **菜单**（K8s 对象）：`kubectl apply` 一个 RuntimeClass，
   `handler` 对上后厨的 runtime 名（见 `impl/runtimeclass-gvisor.yaml`）。
   pod 里写 `runtimeClassName: gvisor` 即点菜；不写用默认 runc。

## 验收（2026-10-07 实机通过）

- `gvisor-demo` 内 `uname -r` = `4.19.0-gvisor`，
  `runc-demo` = 宿主内核——同一集群、两种 sandbox 共存。
- 能画出 kubectl → apiserver → etcd / scheduler → kubelet → containerd → runc
  调用链（见 `notes/学习笔记.md`“关键链条”）。

## 进度

- [x] Session 4-1 ~ 4-6（2026-10-07 实机完成）
- [x] 📦 项目：impl/ 落盘 + tests/TESTS.md + 接入新 runtime 步骤（本 README）
- [x] 多 runtime 集群跑通（runc + runsc 共存）
