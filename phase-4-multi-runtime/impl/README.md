# multi-runtime 集群配置

对应路线：`docs/学习路线.md` Phase 4（Session 4-1 ~ 4-6）。

## 内容

- `runtimeclass-runsc.yaml`：RuntimeClass（`handler: runsc`）
- `demo-runc.yaml` / `demo-runsc.yaml`：同镜像分别用 runc 和 runsc 跑的 Deployment
- `deployment-limits.yaml`：带 `resources.requests/limits` 的示例

## 使用

```bash
# k3s 或 kind 单节点
kubectl apply -f phase-4-multi-runtime/impl/
kubectl get pods  # 确认 runc 和 runsc 的 pod 共存
```

## 依赖

- k3s 或 kind 单节点
- containerd 已注册 `io.containerd.runsc.v1` runtime
