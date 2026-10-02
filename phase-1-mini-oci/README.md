# Phase 1：容器运行时进阶（Go）

> 预计 2–3 周 ｜ 前置：Phase 0 入口自测通过（见 `docs/入口自测.md`）｜ 环境：Linux VM（runc、containerd）+ Mac（Go 1.22+、Docker、skopeo）
> 产出：Go 写的 mini-oci（`impl/`）+ 本章测试全部通过（`tests/TESTS.md`）

## 目录分工

```
phase-1-mini-oci/
├── README.md   # 本章学习计划（你正在看的）
├── impl/       # mini-oci 的 Go 源码：只放 .go 文件，保持干净
├── tests/      # 测试用例（tests/TESTS.md）
└── work/       # 你的草稿纸（git 忽略，不会提交）：
    ├── myimage/        # Session 1-2 手写的 OCI layout
    ├── bundle/         # Session 1-3 手工搭的 runc bundle
    ├── layer.tar       # Session 1-2 打出的 layer
    └── output/         # mini-oci 程序的输出
        ├── images/busybox/   # pull 产物：OCI layout
        └── bundle/           # unpack 产物：rootfs + config.json
```

**规矩**：
- `impl/` 只放源码；所有手写或程序生成的实验文件一律进 `work/`。
- `work/` 是你的草稿纸，**clone 下来是空的**——跟着文档里的 `mkdir` 命令自己建出来，不用从任何地方"获取"。
- `work/` 已在根目录 `.gitignore` 里，不会进仓库；它跟着机器走（Mac 上的 `work/` 不会自动跑到 VM 里）。

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

## Session 1-1｜三个 spec，一人一句话（25 分钟）

**在哪做**：Mac（有 Docker 就行）

**目标**：看到 `docker run` 时，能说出三个 spec 各管哪一段。

**背景**（只看这段，不用查任何文档）：
把一次 `docker run` 想成"点外卖"：
- **distribution-spec** = 外卖平台的配送流程：怎么从商家（registry）把餐送到你手上 → 对应 `docker pull`
- **image-spec** = 餐盒的打包标准：盒子里有几层、每层是什么、标签怎么写 → 对应镜像的 manifest/config/layers
- **runtime-spec** = 餐桌礼仪：餐盒打开后怎么摆、从哪道菜先吃 → 对应 runc 怎么把 bundle 变成一个运行的进程

记住三个字：**拉 → 包 → 跑**。

**动手**（三条命令，每条对应一个 spec）：
1. distribution-spec 管"怎么拉"：
   ```bash
   docker pull busybox:latest
   ```

   看输出：一层层 `Pull complete` —— 这就是 distribution-spec 定义的"传输过程"。
2. image-spec 管"包长什么样"：
   ```bash
   docker image inspect busybox --format='{{.Architecture}} {{.Os}}'
   ```

   预期输出：`arm64 linux`。这就是 image-spec 里 config 规定的字段——镜像是个"有清单的包裹"。
3. runtime-spec 管"怎么跑"：
   ```bash
   docker create --name t1 busybox
   docker ps -a --filter name=t1
   ```

   预期输出：t1 的 STATUS 是 `Created`——容器已经"摆好"了（bundle 就绪），但还没"开吃"（进程没起）。runc 管的就是"从摆好到开吃"这一步。
   ```bash
   docker rm t1   # 收拾干净
   ```

**验证**：合上文档，填这句话——

> `docker pull` 走的是 ______-spec；镜像里"有几层、每层是啥"由 ______-spec 定；runc 按 ______-spec 把 bundle 变成进程。

答案：distribution / image / runtime。全对就过。

**最小 session**：只做第 1 条命令 + 记住"拉→包→跑"三个字。

**下一步**：→ Session 1-2（亲手造一个镜像，验证你理解的 layout 对不对）

> spec 原文链接收进"延伸"了：当字典查，不当课文啃。卡住了才去翻。

---

## Session 1-2｜手写一个最小 OCI 镜像（40 分钟）

**在哪做**：Mac 本机（就是"造文件"，不需要 VM）

**目标**：不用 `docker build`，亲手造出一个能被 Docker 认出来的镜像。

**背景**：OCI layout 就是个目录，结构固定。关键点只有一个：**manifest 和 config 不是顶层文件，它们是 blobs**——文件名就是其内容的 sha256（内容寻址）；顶层只有 `oci-layout` 和 `index.json` 两个"路牌"：

```
work/myimage/
├── oci-layout          # {"imageLayoutVersion": "1.0.0"}，固定写法
├── index.json          # 入口路牌：指向 manifest blob 的 digest
└── blobs/sha256/
    ├── <manifest 的 sha256>   # JSON：指 config + layers
    ├── <config 的 sha256>     # JSON：架构、Entrypoint、diff_ids
    └── <layer 的 sha256>      # tar 包：真正的文件
```

三个 JSON 的关系是一条引用链：`index.json` → manifest →（config + layers），每一环都靠 digest 寻址。

**动手**（Mac，`phase-1-mini-oci/` 目录下）：
1. 准备一个静态编译的 `hello`。**建议**让它支持 `wait` 参数（收到就阻塞不退出），Session 1-3 要用它演示 `running` 状态。

   ```bash
   mkdir -p /tmp/hello && cd /tmp/hello
   go mod init hello     # go build 需要模块上下文；漏了会报 go.mod file not found
   ```

   ```go
   // /tmp/hello/main.go
   package main

   import (
   	"fmt"
   	"os"
   	"os/signal"
   	"syscall"
   )

   func main() {
   	if len(os.Args) > 1 && os.Args[1] == "wait" {
   		// 别用 select{} —— 它会触发 Go 的 deadlock 检测，见「常见坑」
   		ch := make(chan os.Signal, 1)
   		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
   		<-ch
   		return
   	}
   	fmt.Println("hello from OCI")
   }
   ```
   ```bash
   # 交叉编译成静态 arm64 二进制，产物直接落到 repo 根
   GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
     go build -o ~/code/sandbox-study/phase-1-mini-oci/hello .
   file ~/code/sandbox-study/phase-1-mini-oci/hello
   # 期望：ELF 64-bit LSB executable, ARM aarch64, statically linked
   ```
2. 用 Go 打 layer tar 并算 digest（在 `phase-1-mini-oci/` 目录下执行；先 `mkdir -p work`，再把下面存成 `mk-layer.go`，然后 `go run mk-layer.go`）：

```go
package main

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
)

func main() {
	f, err := os.Create("work/layer.tar")
	if err != nil {
		log.Fatal(err)
	}
	tw := tar.NewWriter(f)

	// error 必须接住：丢掉它就会静默打出一个空 layer，见「常见坑」
	data, err := os.ReadFile("hello")
	if err != nil {
		log.Fatal("读不到 hello，先 go build：", err)
	}
	if len(data) == 0 {
		log.Fatal("hello 是 0 字节，先 go build")
	}

	if err := tw.WriteHeader(&tar.Header{Name: "hello", Mode: 0o755, Size: int64(len(data))}); err != nil {
		log.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		log.Fatal(err)
	}
	tw.Close()
	f.Close()

	raw, err := os.ReadFile("work/layer.tar")
	if err != nil {
		log.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	fmt.Printf("sha256:%x  size=%d\n", sum, len(raw))
}
```

   预期输出：`sha256:4f2c…  size=2406912`（一串 hex + 字节数）。

   > `size` 就是 layer 的字节数，取决于你的 `hello` 多大。Go 静态二进制约 2.4 MB，所以是这个量级。
   > **如果你看到 `size=1536` 或更小的数，说明 `hello` 是空的**（`os.ReadFile` 读失败被静默吞了），见「常见坑」。

   > `mk-layer.go` 放哪？`phase-1-mini-oci/` 根下随手放就行（或 `/tmp`），它是草稿，不进 `impl/`，用完删掉。

3. 按"内容寻址"的顺序生成 blobs——**每一步产出的 digest 都是下一步的输入**，这就是内容寻址：

   ```bash
   mkdir -p work/myimage/blobs/sha256
   cd work/myimage   # 后面几步都在这执行，做完记得 cd 回去
   ```

   a. 存 layer blob（文件名 = digest）：
   ```bash
   LAYER_DIGEST=$(shasum -a 256 ../layer.tar | awk '{print $1}')
   cp ../layer.tar blobs/sha256/$LAYER_DIGEST
   ```

   b. 写 config blob（注意 `diff_ids` 填的是 layer 的 digest）：
   ```bash
   cat > blobs/sha256/config.json <<EOF
   {
     "architecture": "arm64",
     "os": "linux",
     "rootfs": {"type": "layers", "diff_ids": ["sha256:$LAYER_DIGEST"]},
     "config": {"Entrypoint": ["/hello"]}
   }
   EOF
   CONFIG_DIGEST=$(shasum -a 256 blobs/sha256/config.json | awk '{print $1}')
   CONFIG_SIZE=$(wc -c < blobs/sha256/config.json | tr -d ' ')
   mv blobs/sha256/config.json blobs/sha256/$CONFIG_DIGEST
   ```

   c. 写 manifest blob：
   ```bash
   cat > blobs/sha256/manifest.json <<EOF
   {
     "schemaVersion": 2,
     "mediaType": "application/vnd.oci.image.manifest.v1+json",
     "config": {
       "mediaType": "application/vnd.oci.image.config.v1+json",
       "digest": "sha256:$CONFIG_DIGEST",
       "size": $CONFIG_SIZE
     },
     "layers": [{
       "mediaType": "application/vnd.oci.image.layer.v1.tar",
       "digest": "sha256:$LAYER_DIGEST",
       "size": $(wc -c < ../layer.tar | tr -d ' ')
     }]
   }
   EOF
   MANIFEST_DIGEST=$(shasum -a 256 blobs/sha256/manifest.json | awk '{print $1}')
   MANIFEST_SIZE=$(wc -c < blobs/sha256/manifest.json | tr -d ' ')
   mv blobs/sha256/manifest.json blobs/sha256/$MANIFEST_DIGEST
   ```

   d. 写顶层两个"路牌"：
   ```bash
   cat > index.json <<EOF
   {
     "schemaVersion": 2,
     "mediaType": "application/vnd.oci.image.index.v1+json",
     "manifests": [{
       "mediaType": "application/vnd.oci.image.manifest.v1+json",
       "digest": "sha256:$MANIFEST_DIGEST",
       "size": $MANIFEST_SIZE
     }]
   }
   EOF
   echo '{"imageLayoutVersion": "1.0.0"}' > oci-layout
   cd ../..   # 回到 phase-1-mini-oci/
   ```

   e. 检查最终结构：
   ```bash
   find work/myimage -type f
   ```

   预期输出：5 个文件——`oci-layout`、`index.json`、`blobs/sha256/<64 位 hex>` × 3。

4. 验证（没装 skopeo 先 `brew install skopeo`）：
   ```bash
   # Colima / Docker Desktop 的 socket 不在 /var/run/docker.sock，
   # 而 skopeo 不读 docker context，必须显式指定，见「常见坑」
   skopeo copy \
     --dest-daemon-host "unix://$HOME/.colima/default/docker.sock" \
     oci:work/myimage docker-daemon:myimage:1.0

   docker run --rm myimage:1.0
   ```

   预期输出：`hello from OCI`；`docker images` 里出现 `myimage:1.0`。

   常驻模式（Session 1-3 要用的姿势）：
   ```bash
   docker run -d --name oci-wait myimage:1.0 wait
   docker ps --filter name=oci-wait        # → Up
   docker stop oci-wait                    # SIGTERM → ExitCode=0
   docker rm oci-wait
   ```

**验证**：`docker images` 里出现 `myimage:1.0`（对应测试 T1-1/T1-2）。

**常见坑**（按"你会看到的报错"组织；每一条都是真踩过的）：

**环境 / 路径类**

- **zsh 卡住不动，没有任何报错**：`cat > file <<EOF` 的收尾 `EOF` **必须顶格**（第 0 列）。前面有一个空格它就不算终结符，zsh 会把后面所有行——包括后面的命令——全当正文吞掉，然后永远等下去。注意 `<<-` 只剥前导 **tab**，不剥空格，所以空格缩进的 `<<-EOF` 照样卡。
  - 救出来：`Ctrl-C`；或手动敲一个顶格的 `EOF` 回车。
  - 卡住时**命令根本没执行**，别以为文件已经写好了。
  - 一劳永逸：别用 heredoc，改用 `printf '%s\n' 'line1' 'line2' > file`，或者直接用编辑器写。
- **`Invalid source name oci:work/my-image: lstat .../work/work: no such file or directory`**：`oci:` 后面的路径是**相对当前目录**的，不是相对仓库根。

  | 你的 cwd | 正确写法 |
  |---|---|
  | `phase-1-mini-oci/` | `oci:work/myimage` |
  | `phase-1-mini-oci/work/` | `oci:myimage` |

  （报错里出现重复的路径段，比如 `work/work`，就是这条。）
- **`failed to connect to the docker API at unix:///var/run/docker.sock`**：Colima / Docker Desktop 的 socket 不在那儿。关键点是 **skopeo 不读 docker context**（`docker context ls` 里那个地址它看不见），而且 **`DOCKER_HOST` 环境变量也不管用**，必须用专用 flag：
  ```bash
  skopeo copy --dest-daemon-host "unix://$HOME/.colima/default/docker.sock" \
    oci:work/myimage docker-daemon:myimage:1.0
  ```
  （`docker-daemon:` 当**源**时同理，用 `--src-daemon-host`。）
- **`writing blob: io: read/write on closed pipe`**：这条**不是**独立错误，是上一条的**下游症状**——skopeo 一边往管道灌 tar，一边 daemon 连不上把管道关了，于是报这个。**看到 closed pipe，先往上翻找 `failed to connect`**，别去查 layer 格式。

**静默错误类（最坑：没有报错，但结果是错的）**

- **`os.ReadFile` 的 error 被丢掉 → 打出空 layer**：`hello` 不存在时它返回空 slice，`mk-layer.go` 于是打出一个**0 字节的 `hello`**。整个 OCI 结构完全合法、`skopeo inspect` 也照收，直到 `docker run` 才炸：
  ```
  exec /hello: exec format error
  ```
  - **症状识别**：`go run mk-layer.go` 输出 `size=1536`（或任何远小于二进制体积的数）。正常应该是 2.4 MB 量级。
  - 二次确认：`tar tvf work/layer.tar` 看 `hello` 那行的大小是不是 0。
  - 修法：照上面 `mk-layer.go` 把每个 error 都接住。**Go 里丢掉 error 就是在给未来的自己埋雷。**
- **改了 layer 就必须从头重造**：digest 是内容寻址，`layer.tar` 变一个字节，`LAYER_DIGEST` → `CONFIG_DIGEST` → `MANIFEST_DIGEST` **整条链全变**。只改一处只会得到一堆对不上的引用。
  - 稳妥做法：`rm -rf work/myimage`，然后 a→b→c→d 原样重跑。**别想着增量改。**
- **`jq` 路径写错时输出 `null`，而且退出码是 0**：`jq -r '.layer[0].digest'`（少了个 `s`，JSON 里是 `layers`）不会报错，只给你一个 `null`——写进文件、拼进 URL，一路静默错下去（`.../blobs/null` → 一个字节都收不到的 `HTTP 000`）。
  - 症状识别：`cat x.digest` 输出 `null`。
  - 修法：用 **`jq -er`**（`-e` 让 null/false 变成非零退出码），路径写错就当场失败，而不是把 `null` 传下去。
  - 通用口诀：**任何"取值"步骤之后，先 `cat` 一眼再用**。这跟上一条「error 被丢掉」是同一类坑。

**Go 编译类**

- **`go: go.mod file not found in current directory or any parent directory`**：`go build` 需要模块上下文，`go mod init hello` 一下即可。（`go run mk-layer.go` 这种「显式指定单个 .go 文件」的写法不受影响，所以它能在没有 go.mod 的目录里跑。）
- **`"os" imported and not used`**：Go 在**编译期**禁止「导入了不用」。`os` 的唯一用途就是 `os.Args`，一旦删掉 `wait` 分支，它就变成孤儿了。
  - 三个同族错误的记忆模板：`imported and not used` = 删导入或补代码；`undefined: X` = 加导入；`declared and not used` = 删变量。
  - 顺带：缩进用 **tab**（gofmt 规范），`gofmt -w main.go` 一把梭。
- **`select {}` 会让程序 panic**：作为**唯一**的 goroutine，Go runtime 的 deadlock 检测会直接干掉进程：
  ```
  fatal error: all goroutines are asleep - deadlock!
  goroutine 1 [select (no cases)]
  ```
  实测：`select {}` ❌ 崩（exit 2）；`for { time.Sleep(time.Hour) }` ✅；信号等待 ✅。
  推荐用**信号等待**（上面 step 1 的写法）——好处是 `docker stop` 发的 SIGTERM 能让它体面退出，Session 1-3 演示 `Up → exited` 正好用得上。

**Docker 报错会骗人**

- **`docker: Error response from daemon: pull access denied for myimage, repository does not exist or may require 'docker login'`**：**九成不是登录问题。**
  - 真正的信号是它上面那行 `Unable to find image 'myimage:1.0' locally`——本地没有，才去 pull。
  - Docker Hub 对「私有仓库你没权限」和「仓库根本不存在」**返回的都是 401**（防止被人探测私有仓库是否存在），所以 daemon 只能把两个原因用 `or` 拼起来丢给你。这句话是「我拿到 401，原因你自己猜」，不是诊断结论。
  - 另外 `myimage` 不含 `/`，Docker 会补全成 `docker.io/library/myimage`（官方 library 命名空间，只放 Docker 自己维护的镜像），你本地那个从没推上去过，当然不在。
  - **判断口诀**：看到 `Unable to find image ... locally`，回头查**上一条命令**（通常是 `skopeo copy` 或 `docker build`）为什么没成功，**别去 `docker login`**。

**结构 / 校验类**

- blob 文件名必须 **exactly** 是 hex digest（**不带** `sha256:` 前缀）；`size` 必须和实际字节数**完全一致**，差 1 个字节就认不出来。
- `architecture` 写错（Apple Silicon 是 `arm64`，不是 `amd64`）→ Docker 报平台不匹配。注意 `go build` 在 Apple Silicon 上默认就是 arm64，和这里一致。
- `diff_ids` 是**未压缩** tar 的 digest；本节用纯 tar（`+tar`，不是 `+tar+gzip`），两者恰好相等，所以少一个坑。一旦改成 gzip，`diff_ids` 就得用**解压后**的 digest。
- macOS 的 `tar` 会往包里塞 AppleDouble（`._*`）伴生文件——`tar tvf` 里看到 `._hello` 别慌；用系统 tar 造测试镜像时 `COPYFILE_DISABLE=1 tar ...` 关掉它。
- `shasum` 是 macOS 自带的；Linux 上对应命令是 `sha256sum`。本节在 Mac 做。
- `go run mk-layer.go` 报 `expected 'package', found 'EOF'` → 文件是空的（内容没写进去），`head mk-layer.go` 检查下；文件名也要和命令里的一致。
- **`oci-layout` 里的 `imageLayoutVersion` 拼错**（例如写成 `iamgeLayoutVersion`）→ 工具报 `invalid version of OCI layout file`。这个字段的值**参与判断**，所以拼错会被抓到；但它本质仍属「拼错 = 静默填零值」，只是恰好有工具替你兜底。
- **macOS 的 `tar` 会给每个文件多打一个 `._<名字>` 伴生文件**（AppleDouble）：APFS 上文件带扩展属性（`com.apple.provenance` 等），macOS `tar` 默认把它们存成额外的 `._*` 条目 —— 于是你解出来的 rootfs 里多出一堆垃圾，条目数也虚高。
  - 修法：`COPYFILE_DISABLE=1 tar -cf layer.tar .`（或 bsdtar 的 `--no-mac-metadata`）。
  - 这正是 **Docker Desktop 构建时要设 `COPYFILE_DISABLE=1`** 的原因 —— 不是 Docker 的怪癖，是 macOS tar 的默认行为。

**自查清单**（跑完后逐条打勾）

```bash
ls -l hello                                   # 不是 0 字节
file hello                                    # ELF ... ARM aarch64, statically linked
tar tvf work/layer.tar                        # hello 那行 size > 0
find work/myimage -type f | wc -l             # 5
skopeo inspect oci:work/myimage               # 能输出 JSON，Architecture=arm64
docker run --rm myimage:1.0                   # hello from OCI
```

**下一步**：→ Session 1-3（拿这个镜像的 rootfs 去喂 runc；那节要进 VM）


---

## Session 1-3｜runc：亲手跑一个 bundle（30 分钟）

**在哪做**：Linux VM（要 root + runc）

**目标**：理解"OCI runtime bundle = rootfs + config.json"，会用 runc 起停容器。

**背景**：runc 是 OCI runtime 的参考实现，职责单一：读 `config.json`，按里面的 namespace/cgroup/rootfs 配置 `clone()` 出一个进程。它不管镜像、不管网络——那些是 containerd 的活。

**动手前（三分钟，把路铺平）**：
1. 起 VM（Mac 上执行，一次就行）：
   ```bash
   brew install lima   # 没装过 lima 才要
   limactl start --name=sandbox template://ubuntu --nested-virt
   ```

   之后每次进 VM（Mac 上执行）：
   ```bash
   limactl shell --start sandbox
   ```

   预期：直接进到 VM 的 shell（没起会自动起）。
2. VM 里装 runc：
   ```bash
   sudo apt update && sudo apt install -y runc
   runc --version
   ```

   预期输出：`runc version 1.x.x`（x 是几无所谓，能打印就行）。
3. 把 Mac 上的 layer.tar 送进 VM（**Mac 上执行**，在 `phase-1-mini-oci/` 目录下——VM 里不会自动有 Mac 的 work/）：
   ```bash
   limactl copy work/layer.tar sandbox:/tmp/layer.tar
   ```

   回 VM 里接住它：
   ```bash
   mkdir -p ~/p1/work/bundle/rootfs && cp /tmp/layer.tar ~/p1/work/layer.tar && cd ~/p1/work/bundle
   ```

**动手**（VM 里，需要 root）：
1. 准备 bundle（接"动手前"第 3 步，当前目录就是 `~/p1/work/bundle`，后面几步都在这执行；VM 里不需要 clone 仓库，Session 1-3 是独立实验）：
   ```bash
   tar -xf ../layer.tar -C rootfs   # 用 Mac 传过来的 layer
   runc spec                        # 生成默认 config.json
   ```
2. 改 `config.json`（复制粘贴整段；heredoc 收尾的 `EOF` 必须顶格，见 Session 1-2 常见坑）：
   ```bash
   command -v python3 >/dev/null || sudo apt install -y python3
   python3 - <<'EOF'
   import json
   p = 'config.json'
   c = json.load(open(p))
   c['process']['args'] = ['/hello']
   assert c['root']['path'] == 'rootfs', c['root']
   json.dump(c, open(p, 'w'), indent=2)
   print('args =', c['process']['args'])
   EOF
   ```

   预期输出：`args = ['/hello']`。如果 assert 挂了，说明 `runc spec` 生成的模板跟预想的不一样，先停下来看一眼 `config.json`，别往下走。
3. 跑起来：
   ```bash
   sudo runc run demo
   ```

   预期输出：你的 hello 打印出来，容器退出。
4. 看"运行中"状态：把 `config.json` 的 `process.args` 改成 `["/hello", "wait"]`（需要你的 hello 支持 `wait` 参数，见 Session 1-2 注），然后：
   ```bash
   sudo runc run -d demo2
   sudo runc list
   ```

   预期输出：
   ```
   ID      PID     STATUS    BUNDLE           CREATED                          OWNER
   demo2   12345   running   .../work/bundle  2026-10-01T01:00:00.000000000Z   root
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
- **`sudo runc run -d demo` 报 `cannot allocate tty if runc will detach without setting console socket`**：`config.json` 里 `process.terminal: true`（`runc spec` 的默认值）意思是「给我一个 TTY」，runc 默认把它接到你当前的 `/dev/tty`；而 `-d` 又说「你去后台，我不管了」—— 后台进程不可能占用前台终端，于是直接报错。
  - 修法①：`sed -i 's/"terminal": true/"terminal": false/' config.json`（简单，日志走 stdout/stderr）。
  - 修法②：给 `--console-socket /path/to/sock`，把控制台交给别人托管 —— **containerd / `ctr run -t` 就是这么干的**。
  - 同一问题的另一面：`docker run` 不带 `-i -t` 时，`Cmd` 是 `sh` 也拿不到 stdin，**读到 EOF 立刻退出**（看起来像「什么都没发生」，退出码还是 0）。
- **`sudo runc delete demo` 报 `container does not exist`**：**不是错**。`runc run`（不加 `-d`）是「创建 + 启动 + **退出后自动清理**」，前台跑完容器已经不存在了。只有 `runc create`、或 `runc run -d` 留下的容器才需要 `delete`。
- `limactl copy` 提示 instance 不存在 → VM 还没建或名字不对：`limactl list` 看实际名字，对上再拷。
- `tar -xf ../layer.tar` 报找不到文件 → "动手前"第 3 步的 `cp` 没做或目录错了，`ls ~/p1/work/layer.tar` 确认。

**自查清单**（跑完后逐条打勾）：

```bash
ls -l ~/p1/work/layer.tar                  # 传进来了，不是 0 字节
ls ~/p1/work/bundle/rootfs/hello           # 解出来了
grep -A2 '"args"' ~/p1/work/bundle/config.json  # 看到 "/hello"
sudo runc run demo                         # hello from OCI，容器退出
```

**下一步**：→ Session 1-4（runc 上面那层：containerd）

---

## Session 1-4｜containerd + ctr（35 分钟）

**在哪做**：Linux VM

**目标**：会用 `ctr` 拉镜像、跑容器、切 namespace；理解 containerd 是 runc 的"经纪人"。

**背景**：containerd 负责镜像管理、容器生命周期、快照、事件，对外暴露 gRPC API。`ctr` 是它自带的调试 CLI（难用但直接）。docker CLI 背后调的也是 containerd（经由 dockerd）。

**动手**（VM 里；进 VM：`limactl shell --start sandbox`，VM 的起法见 Session 1-3「动手前」）：
1. 装好并确认 containerd 在跑：
   ```bash
   command -v ctr >/dev/null || sudo apt install -y containerd
   sudo ctr version
   ```

   预期输出：能打印出 containerd 版本号（连不上 daemon 会直接报错，见常见坑）。
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
- `ctr: failed to dial "/run/containerd/containerd.sock"` → daemon 没在跑：`sudo systemctl start containerd`，没装 systemd 的环境直接 `sudo containerd &` 另起一个终端跑。

**自查清单**（跑完后逐条打勾）：

```bash
sudo ctr version                    # 能打印版本
sudo ctr image ls                   # 看到 busybox:latest
sudo ctr -n demo containers list     # demo 里有容器；不加 -n 看不到它
```

**下一步**：→ Session 1-5（镜像是怎么"构建"出来的：BuildKit）

---

## Session 1-5｜BuildKit：看懂镜像是怎么「构建」出来的（35 分钟）

**在哪做**：Mac（有 Docker 就行）

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

（heredoc 收尾的 `EOF` 必须顶格，见 Session 1-2 常见坑——这里 `<<'EOF'` 是一样的写法。）

**验证**：口头回答 BuildKit 多解决了什么（并发构建 + 细粒度缓存；经典 builder 串行等层）。

**常见坑**：
- 忘加 `--privileged` → buildkitd 起不来。
- 第一次拉 `moby/buildkit:master` 镜像很慢，耐心等。

**自查清单**（跑完后逐条打勾）：

```bash
ls -l /tmp/bk/Dockerfile
docker image inspect moby/buildkit:master --format='{{.Architecture}}'  # 拉下来了，第一次慢
# 构建日志最后看到 writing image sha256:… done
```

**下一步**：→ 📦 项目 mini-oci（把本章全串起来）

---

## 📦 项目：mini-oci（Go，2–3 个 session）

**状态：已完成**（2026-10-02，实机跑通，T1-1~T1-3 通过）。下面是代码导览，不是作业——读代码时对照着看。

把 Session 1-2 的手工作业写成了真正的工具。接口：

```bash
# 以下都在 phase-1-mini-oci/ 目录下执行（go.work 已配好，go run ./impl 能直接跑）
go run ./impl pull busybox:latest     # 拉 manifest + layers → work/output/images/busybox（OCI layout）
go run ./impl unpack busybox:latest   # work/output/images/busybox → work/output/bundle（rootfs + config.json）
go run ./impl run busybox:latest      # pull + unpack + runc run 一条龙（需 root，见下）
```

**代码导览**（`impl/`，约 1000 行）：

| 文件 | 干什么 |
|---|---|
| `registry.go` | registry HTTP 客户端：401 → 按 `WWW-Authenticate` 去 token 服务换 token，重发请求 |
| `pull.go` | 内容寻址落盘 + digest 校验 |
| `unpack.go` | 解 gzip/tar、硬链接、whiteout、diff_id 校验；`prepareTarget` 处理层间覆盖（见下） |
| `spec.go` | image config → runtime-spec `config.json` |
| `run.go` | pull + unpack + `runc run` 一条龙，跑完 `runc delete` 清理 |
| `layout.go` | 临时文件 → 校验 → 改名（落盘不留半截文件） |
| `main.go` | 子命令分发 |

**实现约定**（测试按此验收）：
- 所有输出一律写进 `work/output/`，不污染 `impl/`。
- 镜像名映射：`busybox:latest` → `work/output/images/busybox`（去掉 `library/` 前缀和 tag）。
- `run` 需要 root 建 namespace：先 `go build -o work/mini-oci ./impl`，再 `sudo work/mini-oci run busybox:latest`（sudo 不改变当前目录，相对路径照常工作）。
- `run` 需要 runc（Linux only）：`pull`/`unpack` 在 Mac 就能做，`run` 那步去 VM 里做（work/ 跟着机器走，把 `work/output` 拷过去，或在 VM 里重跑一次 pull/unpack 也行）。

**Registry API 流程**（已实现，留作对照）：

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
- 官方镜像在 `library/` 下：`busybox` → `library/busybox`；`docker.io/` 前缀要剥掉。
- manifest 可能是 index（多架构）：按 `platform.architecture == "arm64"` 挑一个，再取它的 manifest。注意 index 里 attestation 条目的平台是 `unknown/unknown`（**不是没有 `platform` 字段**），所以判据要用 os/arch 的**值**，不能只判断 `platform` 是否为 nil。
- **`Accept` 头拼错不会报错，只会静默降级**（例如写成 `.../v1.idex.v1+json`）：registry 对不认识的类型只是忽略。所以判断「拿到了什么」要看响应里的 `mediaType`，而不是你请求了什么。
- **token 5 分钟就过期**：换 token 的响应里 `expires_in: 300`。过期后再请求收到 `401` + `www-authenticate: …,error="invalid_token"`。正确姿势是「**任何请求收到 401 就换一次 token 再重发**」，而不是启动时拿一次用到死。
- **blob 不在 registry 上**：`GET /v2/<repo>/blobs/<digest>` 返回 **307**，`location` 指向 CDN（CloudFront）上带签名的临时地址。
  - `curl` **默认不跟随**重定向 —— 不加 `-L`，你保存到的是那个 **0 字节**的重定向响应体，却以为「下载成功」了。
  - Go 的 `http.Client` **默认跟随**（最多 10 跳），所以代码里根本看不到这一步。
  - 跨域重定向时 curl/Go 都会**丢掉 `Authorization` 头** —— 没关系：CDN 靠 URL 里的 `Signature` 鉴权，不需要 token。
- **本地 `index.json` 只能引用「本地真实存在」的 manifest**：远端 index 可能列了 17 条（8 个平台 + 9 个 attestation），而你只下了 arm64 那一份字节 —— 本地 index 必须**自己造**、只放真正存下来的那一个，否则 `skopeo`/`docker` 会去找不存在的 blob（`undefined blob`）。
- **层叠加时上层可以在同一路径换类型**（普通文件 → 硬链接 → 符号链接 → 目录）：新增前必须先拆掉旧的，否则报 `symlink … file exists`。**尤其别对已存在的硬链接直接截断写** —— 那会改到被链接的文件本体（例如 `/bin/busybox` 的字节）。正确姿势是「先 unlink，再 `O_CREATE|O_EXCL` 新建」。
  - 单层镜像（busybox）永远暴露不了这个问题；拿 `python:3.12-alpine`（4 层）这类真实镜像才能测出来。
- **内容寻址缓存**：blob 已存在时可以直接跳过下载，判断只值一次 `os.Stat` —— 因为名字就是内容的 sha256，而 `writeBlob` 的「先写临时文件、校验、再 rename」保证了「文件以正式名字存在」等价于「内容正确」。
  - 但 **`tag → digest` 的映射不能缓存**：tag 是可变指针，缓存它就永远拉不到更新（而且无法察觉）。
  - 口诀：**digest 是地址（可以存货），tag 是名字（必须去问）。**

**真实镜像才暴露的 bug**（`638aa50` 修的，值得细看）：
`python:3.12-alpine` 的 layer 2 把 `bin/tar` 从硬链接换成了符号链接——OCI 层叠加语义允许上层在同一路径换文件类型，原来的 `os.Symlink` 直接报 `file exists`。修法：新建前 `Lstat + RemoveAll`（`prepareTarget`）；普通文件用 `O_CREATE|O_EXCL` 且先 unlink，否则截断写会改到硬链接本体的字节（比如 `/bin/busybox`）。**"层怎么叠"正是 Phase 2 overlayfs 的主题**，到时候回来看这段代码。

**验收**（对应 `tests/TESTS.md` T1-1 ~ T1-3，已通过）：
- `go run ./impl run busybox:latest` 能跑起来
- `go vet ./...` 无报错
- `git status` 干净：`work/` 下的东西没有混进仓库

---

## 测试

完整测试用例见 [tests/TESTS.md](tests/TESTS.md)（T1-1 ~ T1-6）。每节的"验证"就是对应测试的预演。

## 延伸（可选，不阻塞）

- [`tools/`](tools/README.md)：**手工版参照物**。`handpull.sh` 用 curl+jq 把 pull 再做一遍（产物与 `impl/` 逐字节相同，可 `diff -r` 对拍）；`mkmultilayer.sh` 离线造一个带 whiteout 的 3 层镜像，用来验证 `unpack` 的层叠加。
- spec 原文（当字典查，不当课文啃）：[image-spec](https://github.com/opencontainers/image-spec) / [runtime-spec](https://github.com/opencontainers/runtime-spec) / [distribution-spec](https://github.com/opencontainers/distribution-spec)
- `ctr -n k8s.io containers list`：看 k8s 在 containerd 里留下的容器
- [containerd 入门实操](https://github.com/mukappalambda/go-examples/blob/HEAD/container/containerd/getting-started.md)
- [BuildKit rootless 文档](https://github.com/moby/buildkit/blob/HEAD/docs/rootless.md)（只看感兴趣的部分）
