# Phase 4 测试用例

在 k3s 单节点（WSL2）执行。前置：containerd 后厨已注册 runsc
（`config.toml.tmpl` 模板 + `systemctl restart k3s`，见 README）。
2026-10-07 实机全部通过。

| 编号 | 操作 | 预期结果 |
|---|---|---|
| T4-1 | `kubectl apply -f impl/runtimeclass-gvisor.yaml`；`kubectl get runtimeclass` | 看到 `gvisor`，HANDLER=runsc |
| T4-2 | `kubectl apply -f impl/pod-gvisor-demo.yaml`；`kubectl exec gvisor-demo -- uname -r` | pod Running；输出 `4.19.0-gvisor`（gVisor 假内核） |
| T4-3 | `kubectl apply -f impl/pod-runc-demo.yaml`；`kubectl exec runc-demo -- uname -r` | pod Running；输出宿主内核版本 |
| T4-4 | `kubectl get pods -o wide` | 两个 pod 同一集群共存，IP 都在 `10.42.0.0/24` 段 |

**备注**：`kubectl get runtimeclass` 里 kata/wasm 那 30 多个菜单是 3-4 chart
留下的，但后厨（containerd）只认 `config.toml` 里的 `runtimes.*`——
这版 k3s 没加载 `config-v3.toml.d/` drop-in（实测 `grep -c kata config.toml` = 0），
菜单≠可用；加 runtime 走 `config.toml.tmpl` 模板。
