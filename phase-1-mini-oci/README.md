# Phase 1：容器运行时进阶（Go）

> 预计 2–3 周 ｜ 前置：Phase 0 入口自测通过（见 `docs/入口自测.md`）｜ 环境：Linux VM（runc、containerd、skopeo）+ Mac（Go 1.22+）
> 产出：Go 写的 mini-oci（`impl/`）+ 本章测试全部通过（`tests/TESTS.md`）

## 这一章要回答的问题

学完这一章，应该能不假思索地回答：

1. 镜像到底是什么？（不是"一个压缩包"，而是 manifest + config + layers 三件套）
2. `docker run busybox` 按下回车后，依次发生了什么？
3. docker / containerd / runc 三者到底谁在干活？（谁调谁？）

如果现在答不上来，很正常——这一章就是干这个的。

## 全景：一条链看懂 Phase 1

```
registry ──pull──▶ OCI layout ──unpack──▶ bundle ──runc──▶ container
 (远端)        (image-spec)      (rootfs+config.json)   (runtime-spec)  (一个进程)
                    ▲ Session 1-2          ▲ Session 1-3
```

- **image-spec**：镜像在磁盘/网络上长什么样（manifest、config、layer tar）
- **runtime-spec**：容器运行时怎么把 bundle 变成进程（`config.json` + `rootfs/`）
- **distribution-spec**：怎么从 registry 拉镜像（HTTP API）
- **runc**：OCI runtime 的参考实现，只做一件事：按 `config.json` 起进程
- **containerd**：runc 的"经纪人"：管镜像、管容器生命周期，对外提供 API；`ctr` 是它的调试 CLI
- **docker**：containerd 的"前台"：UX + 构建（BuildKit）+ 网络/卷插件

记住这张图，后面每节都是在给这条链补细节。

---

## Session 1-1｜OCI 三件套：只看结构，不读全文（30 分钟）

**目标**：说出 image-spec / runtime-spec / distribution-spec 各管什么，能默写出上面的链。

**背景**：OCI 是容器界的"普通话"。2015 年 Docker 把容器格式捐出来成立 OCI，从此 runc 跑的 bundle、containerd 拉的镜像都讲同一种格式。不需要读 spec 全文，知道"谁管哪段"就行。

**动手**：
1. 打开 [OCI image-spec README](https://github.com/opencontainers/image-spec/blob/HEAD/README.md)，只看目录结构，找到 `manifest.md`、`config.md`、`layout.md` 三个文件名——它们就是三件套。
2. 打开 [OCI runtime-spec README](https://github.com/opencontainers/runtime-spec/blob/HEAD/README.md)，找到 `config.md`（bundle 长什么样）。
3. 在纸上默写一遍全景图，不看上面。

**验证**：合上页面，用一句话向自己解释"镜像 → bundle → 容器"这条链。说不出来就再看一遍图，最多 5 分钟。

**最小 session**：只做第 1 步 + 默写全景图。

**下一步**：→ Session 1-2（亲手造一个镜像，验证你理解的 layout 对不对）

---

## Session 1-2｜手写一个最小 OCI 镜像（40 分钟）

**目标**：不用 `docker build`，亲手造出一个能被 runc/Docker 认出来的镜像。

**背景**：OCI layout 就是个目录，结构固定：

```
myimage/
├── oci-layout          # {"imageLayoutVersion": "1.0.0"}
├── index.json          # 入口：指向 manifest
└── blobs/sha256/
    ├── <manifest 的 sha256>
    ├── <config 的 sha256>
    └── <layer 的 sha256>   # 文件名就是 digest，内容寻址
```

`index.json` 骨架：

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [{
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "digest": "sha256:<...>",
    "size": 1234
  }]
}
```

manifest 骨架：

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:<...>", "size": 123
  },
  "layers": [{
    "mediaType": "application/vnd.oci.image.layer.v1.tar",
    "digest": "sha256:<...>", "size": 456
  }]
}
```

config 骨架（注意 `diff_ids` 是**未压缩** layer tar 的 digest）：

```json
{
  "architecture": "arm64",
  "os": "linux",
  "rootfs": {"type": "layers", "diff_ids": ["sha256:<...>"]},
  "config": {"Entrypoint": ["/hello"]}
}
```

**动手**（Go）：
1. 准备一个静态编译的 `hello`（`go build` 一个打印 hello 的程序，或用 busybox）。
2. 用 Go 打 layer tar 并算 digest（核心就这几行）：

```go
package main

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"os"
)

func main() {
	f, _ := os.Create("/tmp/layer.tar")
	tw := tar.NewWriter(f)
	data, _ := os.ReadFile("hello")
	tw.WriteHeader(&tar.Header{Name: "hello", Mode: 0o755, Size: int64(len(data))})
	tw.Write(data)
	tw.Close()
	f.Close()

	raw, _ := os.ReadFile("/tmp/layer.tar")
	sum := sha256.Sum256(raw)
	fmt.Printf("sha256:%x  size=%d\n", sum, len(raw))
}
```

预期输出：`sha256:4f2c…  size=10240`（一串 hex + 字节数）。

3. 按上面的骨架手写 `config.json`、`manifest.json`、`index.json`、`oci-layout`，把 blobs 按 digest 放进 `blobs/sha256/`。
4. 验证：
   ```bash
   skopeo copy oci:/path/to/myimage docker-daemon:myimage:1.0
   docker run --rm myimage:1.0
   ```

   预期输出：看到你的 `hello` 打印出来；`docker images` 里出现 `myimage:1.0`。

**验证**：`docker images` 里出现 `myimage:1.0`（对应测试 T1-1/T1-2）。

**常见坑**：
- `architecture` 写错（ARM VM 里是 `arm64`，不是 `amd64`）→ Docker 报平台不匹配。
- blob 文件名必须 exactly 是 hex digest（不带 `sha256:` 前缀），`size` 必须和实际字节数一致，差 1 个字节都认不出来。
- `diff_ids` 是未压缩 tar 的 digest；本节用纯 tar（`+tar` 不是 `+tar+gzip`），两者一致，少个坑。

**下一步**：→ Session 1-3（拿这个镜像的 rootfs 去喂 runc）

---

## Session 1-3｜runc：亲手跑一个 bundle（30 分钟）

**目标**：理解"OCI runtime bundle = rootfs + config.json"，会用 runc 起停容器。

**背景**：runc 是 OCI runtime 的参考实现，职责单一：读 `config.json`，按里面的 namespace/cgroup/rootfs 配置 `clone()` 出一个进程。它不管镜像、不管网络——那些是 containerd 的活。

**动手**（VM 里，需要 root）：
1. 准备 bundle：
   ```bash
   mkdir -p bundle/rootfs && cd bundle
   tar -xf /tmp/layer.tar -C rootfs   # 用 Session 1-2 的 layer
   runc spec                          # 生成默认 config.json
   ```
2. 改 `config.json`：`process.args` → `["/hello"]`，确认 `root.path` 是 `"rootfs"`。
3. 跑起来：
   ```bash
   sudo runc run demo
   ```

   预期输出：你的 hello 打印出来，容器退出。
4. 看状态：
   ```bash
   sudo runc run -d demo2 -- /bin/sh -c 'sleep 100'
   sudo runc list
   ```

   预期输出：
   ```
   ID      PID     STATUS    BUNDLE        CREATED                       OWNER
   demo2   12345   running   /path/bundle  2026-10-01T01:00:00.000000Z   root
   ```
   ```bash
   sudo runc kill demo2 KILL
   sudo runc delete demo2
   ```

**验证**：口头回答 `runc create` 和 `runc run` 的区别（create 只创建不启动，run = create + start；对应测试 T1-6）。

**常见坑**：
- 忘加 `sudo` → 各种 permission denied（runc 要建 namespace）。
- `rootfs` 里没有 `/bin/sh` 却配了 `args: ["/bin/sh"]` → `no such file`；用你自己的 `/hello` 最稳。
- `config.json` 的 `linux.namespaces` 缺了 `mount` → rootfs 挂载失败。

**下一步**：→ Session 1-4（runc 上面那层：containerd）

---

## Session 1-4｜containerd + ctr（35 分钟）

**目标**：会用 `ctr` 拉镜像、跑容器、切 namespace；理解 containerd 是 runc 的"经纪人"。

**背景**：containerd 负责镜像管理、容器生命周期、快照、事件，对外暴露 gRPC API。`ctr` 是它自带的调试 CLI（难用但直接）。docker CLI 背后调的也是 containerd（经由 dockerd）。

**动手**：
1. 确认 containerd 在跑：`sudo systemctl status containerd`（没装服务的直接 `sudo containerd &`）。
2. 拉镜像、跑容器：
   ```bash
   sudo ctr image pull docker.io/library/busybox:latest
   sudo ctr run --rm -t docker.io/library/busybox:latest t1 /bin/sh
   ```

   预期：在容器 sh 里 `ls /` 看到 busybox 的根文件系统，`exit` 退出。
3. 玩 namespace（**注意：这是 containerd 的元数据隔离，不是 Linux namespace**）：
   ```bash
   sudo ctr namespace create demo
   sudo ctr -n demo image pull docker.io/library/busybox:latest
   sudo ctr containers list        # 看不到 demo 里的
   sudo ctr -n demo containers list
   ```

   预期：`default` namespace 下看不到 `demo` 里的容器，反之亦然。

**验证**：不看笔记说出 `ctr` 和 `docker` 命令的对应关系（`ctr image pull` ≈ `docker pull`，`ctr run` ≈ `docker run`；对应测试 T1-5）。

**常见坑**：
- containerd 的 namespace 和 Linux namespace 是两码事——前者只是 containerd 内部给容器/镜像分组的标签。
- k8s 用的 namespace 叫 `k8s.io`：`sudo ctr -n k8s.io containers list` 能看到 k8s 拉起的容器（延伸玩法）。

**下一步**：→ Session 1-5（镜像是怎么"构建"出来的：BuildKit）

---

## Session 1-5｜BuildKit：看懂镜像是怎么「构建」出来的（35 分钟）

**目标**：理解 LLB（构建图）和缓存；说出 BuildKit 相对 `docker build` 多解决了什么。

**背景**：经典 `docker build` 是串行的：一条 Dockerfile 指令 → 一层 → 等上一步完成。BuildKit 把 Dockerfile 编译成 LLB（一张 DAG），无关步骤并发执行，缓存粒度细到文件级。`docker buildx` 背后就是它。

**动手**（daemonless，一行命令，不装 daemon）：
```bash
mkdir -p /tmp/bk && cd /tmp/bk
cat > Dockerfile <<'EOF'
FROM busybox
RUN echo hello > /hello.txt
EOF
docker run --rm --privileged -v $PWD:/tmp/work --entrypoint buildctl-daemonless.sh \
  moby/buildkit:master build --frontend dockerfile.v0 \
  --local context=/tmp/work --local dockerfile=/tmp/work
```

预期输出：一堆 `#1 [internal] load build definition` / `#n DONE` 日志，最后出现 `writing image sha256:… done`。

**验证**：口头回答 BuildKit 多解决了什么（并发构建 + 细粒度缓存；经典 builder 串行等层）。

**常见坑**：
- 忘加 `--privileged` → buildkitd 起不来。
- 第一次拉 `moby/buildkit:master` 镜像很慢，耐心等。

**下一步**：→ 📦 项目 mini-oci（把本章全串起来）

---

## 📦 项目：mini-oci（Go，2–3 个 session）

把 Session 1-2 的手工作业写成真正的工具。接口：

```bash
go run ./impl pull busybox:latest ./images/busybox   # 拉 manifest + layers，存 OCI layout
go run ./impl unpack ./images/busybox ./bundle       # 解包成 rootfs + 生成 bundle config.json
go run ./impl run busybox:latest                     # pull + unpack + runc run 一条龙
```

**建议实现顺序**（每步都可独立验证）：
1. `pull`：先调通 registry HTTP API（见下），能把 blobs 下到本地就算成。
2. `unpack`：解 tar + 按 runtime-spec 写 `config.json`（复用 Session 1-3 的经验）。
3. `run`：拼起来，用 `os/exec` 调 `runc run`。

**Registry API 流程**（Docker Hub，第一次调通最花时间，值得）：

```
1. GET https://registry-1.docker.io/v2/
   → 401，WWW-Authenticate: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/busybox:pull"
2. GET https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/busybox:pull
   → {"token": "..."}
3. GET https://registry-1.docker.io/v2/library/busybox/manifests/latest
   Header: Authorization: Bearer <token>
           Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json
4. GET https://registry-1.docker.io/v2/library/busybox/blobs/<digest>   # 逐个下 layer
```

**常见坑**：
- 官方镜像在 `library/` 下：`busybox` → `library/busybox`。
- manifest 可能是 index（多架构）：按 `platform.architecture == "arm64"` 挑一个，再取它的 manifest。
- token 有有效期，401 了就重新拿，别长期缓存。

**验收**（对应 `tests/TESTS.md` T1-1 ~ T1-3）：
- `go run ./impl run busybox:latest` 能跑起来
- `go vet ./...` 无报错

---

## 测试

完整测试用例见 [tests/TESTS.md](tests/TESTS.md)（T1-1 ~ T1-6）。每节的"验证"就是对应测试的预演。

## 延伸（可选，不阻塞）

- `ctr -n k8s.io containers list`：看 k8s 在 containerd 里留下的容器
- [containerd 入门实操](https://github.com/mukappalambda/go-examples/blob/HEAD/container/containerd/getting-started.md)
- [BuildKit rootless 文档](https://github.com/moby/buildkit/blob/HEAD/docs/rootless.md)（只看感兴趣的部分）
