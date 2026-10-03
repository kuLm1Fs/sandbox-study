# tools/：手工版参照物

这里的脚本是**用 shell 把同样的事情再做一遍**。它们不是产品代码，是两种用途：

| 用途 | 怎么用 |
|---|---|
| **对着读**：理解 `impl/` 里每个函数在干什么 | 脚本里每一步都标着它对应 mini-oci 的哪个函数 |
| **当试金石**：验证 `impl/` 的实现对不对 | `diff -r` 两个产物，逐字节比 |

**规矩**：脚本产生的所有东西都在 `../work/`（git 忽略），仓库里只放脚本本身。

---

## `handpull.sh` —— 手搓版 `pull`

只用 `curl` + `jq` + `shasum`，把一个镜像从 registry 拉到本地 OCI layout：

```bash
./tools/handpull.sh
# 产物：work/output/shell-busybox/
```

它和 `go run ./impl pull busybox:latest` 的产物**逐字节相同**：

```bash
diff -r work/output/shell-busybox work/output/images/busybox   # 应该没有任何输出
```

**读它的时候重点关注三件事**（都是 Go 代码里"看不见"的）：

1. **401 → 换 token → 重发**：认证是流程的一部分，不是错误分支；
2. **blob 走 307 到 CDN**：`curl` 必须加 `-L`，而 Go 的 `http.Client` 默认跟随——所以 `impl/registry.go` 里看不到这一步；
3. **先写临时文件、校验、再改名**：对应 `writeBlob`，这是"文件名存在 ≡ 内容正确"的来源。

## `mkmultilayer.sh` —— 造一个多层测试镜像

不联网，手工搭一个 3 层镜像，专门用来验证 `unpack` 的**层叠加**和 **whiteout**：

```
layer 1: a.txt  b.txt
layer 2: a.txt(覆盖)  c.txt
layer 3: .wh.b.txt      ← 删除标记
```

```bash
./tools/mkmultilayer.sh
go run ./impl unpack whiteout-demo:latest
ls -a work/output/bundle/rootfs     # 应该只有 a.txt 和 c.txt
```

**为什么需要它**：单层镜像（busybox）永远测不出层叠加的 bug。`impl/unpack.go` 的
`prepareTarget`（上层换文件类型时必须先拆旧的）就是被 `python:3.12-alpine` 这种真实
多层镜像逼出来的——这个脚本让你**离线**也能复现同类场景。

**两个坑写在这里，免得再踩**：

- **macOS 的 `tar` 默认会打 AppleDouble 伴生文件**（`._a.txt` 这种），因为 APFS 上文件带扩展
  属性。脚本里用 `COPYFILE_DISABLE=1` 关掉（Docker Desktop 也是这么干的）。
- **`diff_ids`（未压缩 tar）和 layer blob 的 digest（压缩后）是两个值**，脚本里两组都算了，
  对照着看就明白 `impl/unpack.go` 为什么要解压之后再校验。
