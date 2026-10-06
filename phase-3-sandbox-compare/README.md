# Phase 3｜虚拟化与轻量沙盒（3–4 周）

> 环境：Linux VM（ARM64，`/dev/kvm` 存在，要 root）｜ 前置：Phase 2 完成
>
> **目标**：亲手跑通 KVM / Firecracker / gVisor / Kata，输出一张"隔离边界 / 启动速度 / 内存开销 / 适用场景"对比表。

## 入口自测（15 分钟，先做这个）

做 `docs/入口自测.md` 的 Phase 3 五道题。判定：全对可压缩/跳过已掌握的 session；错 3 题以上按计划完整学。

## 进度

- [ ] Session 3-1｜KVM：起一台 ARM64 VM + 快照
- [ ] Session 3-2｜Firecracker：第一个 microVM
- [ ] Session 3-3｜gVisor：runsc 跑容器
- [ ] Session 3-4｜Kata：k3s 里跑 Kata pod
- [ ] Session 3-5｜bench.go：三方案对比评测
- [ ] RESULTS.md 对比表 + 3 条结论

---

## Session 3-1｜KVM：起一台 ARM64 VM + 快照（40 分钟）

**在哪做**：Linux VM（要 root，`/dev/kvm` 存在）

**目标**：用 cloud image 10 分钟起一台 ARM64 VM，打快照、恢复，验证回到快照点。

**前置自检**：

```bash
ls /dev/kvm && echo kvm-ok
uname -m   # 预期：aarch64
```

**动手**：

1. 装包：

```bash
sudo apt update && sudo apt install -y qemu-kvm libvirt-daemon-system virtinst genisoimage
virsh --version   # 预期：有版本号输出
```

2. 下 Ubuntu ARM64 cloud image：

```bash
cd /tmp && wget -q --show-progress \
  https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-arm64.img \
  -O demo.qcow2
qemu-img info demo.qcow2 | grep "virtual size"   # 预期：virtual size: 2.2G 之类
sudo cp demo.qcow2 /var/lib/libvirt/images/demo.qcow2
```

3. 做 cloud-init seed（设登录密码，省掉装系统向导）：

```bash
mkdir -p /tmp/seed && cd /tmp/seed
cat > user-data <<'EOF'
#cloud-config
password: demo123
chpasswd: { expire: False }
ssh_pwauth: True
EOF
cat > meta-data <<'EOF'
instance-id: demo
local-hostname: demo
EOF
genisoimage -output /tmp/seed.iso -volid cidata -joliet -rock user-data meta-data
ls -lh /tmp/seed.iso   # 预期：几百 KB 的 iso
```

4. 起 VM（`--import` 直接用现成磁盘，不走安装）：

```bash
sudo virt-install --name demo --ram 2048 --vcpus 2 \
  --disk path=/var/lib/libvirt/images/demo.qcow2 \
  --disk path=/tmp/seed.iso,device=cdrom \
  --os-variant ubuntu22.04 --network network=default \
  --graphics none --console pty,target_type=serial --import
```

预期：cloud-init 跑完后出现 `demo login:`。用 `demo` / `demo123` 登录。退出 console 按 `Ctrl+]`。

5. 打快照、做标记、恢复、验证：

```bash
sudo virsh list                        # 预期：demo running
sudo virsh snapshot-create-as demo snap1 --description "first boot"
sudo virsh snapshot-list demo          # 预期：看到 snap1
sudo virsh console demo                # 登录后执行：touch /tmp/after-snap，然后 Ctrl+] 退出
sudo virsh shutdown demo
sudo virsh snapshot-revert demo snap1
sudo virsh start demo
sudo virsh console demo                # 登录后执行：ls /tmp/after-snap
```

预期：`ls: cannot access '/tmp/after-snap'`——文件没了，证明磁盘回到了快照点。

**刚才发生了什么**：KVM 是内核模块（`/dev/kvm` 就是它的设备文件），QEMU 是用户态的设备模拟；`virsh`/`virt-install` 只是 libvirt 的前端。cloud image 是预装好的磁盘，cloud-init 在首次启动时注入密码——整套流程 10 分钟，不用点安装向导。快照 = qcow2 磁盘在某个时刻的增量点 + VM 配置。

**验证**：不看笔记说出 KVM / QEMU / libvirt 三者的关系（一句话）。

**自查清单**：

```bash
sudo virsh list              # demo running
sudo virsh snapshot-list demo  # snap1 在
ls -lh /var/lib/libvirt/images/demo.qcow2
```

**常见坑**：（待实机补充）

**下一步**：→ Session 3-2（Firecracker：比这台 VM 轻 100 倍的 microVM）

---

## Session 3-2｜Firecracker：第一个 microVM（40 分钟）

**在哪做**：Linux VM（要 root，`/dev/kvm` 存在）

**目标**：跑通官方 getting-started，ssh 进 microVM，`uname -r` 和宿主机不一样。

**前置自检**：

```bash
ls /dev/kvm && echo kvm-ok
uname -m   # 预期：aarch64（下 aarch64 的二进制和 kernel）
```

**动手**（跟着[官方 getting-started](https://github.com/firecracker-microvm/firecracker/blob/main/docs/getting-started.md) 走，下面的骨架是步骤清单，**版本号和下载链接以文档为准**，不要背）：

1. 下 Firecracker 二进制：去 [releases 页](https://github.com/firecracker-microvm/firecracker/releases) 拿最新版号，设成 `FC_VER`，下 aarch64 包：

```bash
FC_VER=<releases 页看到的最新版>   # 以页面为准，不要猜
wget https://github.com/firecracker-microvm/firecracker/releases/download/${FC_VER}/firecracker-${FC_VER}-aarch64.tgz
tar xzf firecracker-${FC_VER}-aarch64.tgz
./release-${FC_VER}-aarch64/firecracker-v${FC_VER}-aarch64 --version   # 预期：打印版本号
```

2. 下 kernel + rootfs：按 getting-started 文档里的 **aarch64** 链接拿 `vmlinux` 和 `ubuntu-22.04.ext4`（文档会给地址，复制粘贴）。

3. 配 tap 网络（宿主机侧）：

```bash
sudo ip tuntap add tap0 mode tap
sudo ip addr add 172.16.0.1/24 dev tap0
sudo ip link set tap0 up
ip addr show tap0 | grep 172.16.0.1   # 预期：看到地址
```

4. 起 firecracker 进程（一个终端），再用 `curl --unix-socket` 按顺序放配置：`boot-source` → `drives` → `network-interfaces` → `machine-config` → `actions`（`InstanceStart`）。字段照文档抄，`vcpu_count`/`mem_size_mib` 先给 `2` / `512`。

5. 进 microVM（文档默认给了 serial console 输出和 ssh `root`/`root`）：

```bash
ssh -o StrictHostKeyChecking=no root@172.16.0.2
uname -r   # 预期：和宿主机的 uname -r 不一样（它是 guest 内核）
```

**卡住降级**：tap 网络那节先跳过——只要 serial console 里看到启动日志、能敲命令，就算过，ssh 下次再说。

**刚才发生了什么**：Firecracker 是一个进程，VMM + API server 二合一；所有配置都是一次性 `curl` 扔进 unix socket 的 JSON。microVM = KVM 硬件虚拟化 + 极简设备模型（只有串口、网卡、磁盘几样），所以毫秒级启动。

**验证**：说出 Firecracker 和 Session 3-1 那台 QEMU VM 的最大区别（一句话：极简设备模型，启动快 100 倍量级）。

**自查清单**：

```bash
ps aux | grep -v grep | grep firecracker   # 进程在
# microVM 里：uname -r 与宿主机不同
```

**常见坑**：（待实机补充）

**下一步**：→ Session 3-3（gVisor：不走硬件虚拟化的另一条路）

---

## Session 3-3｜gVisor：runsc 跑容器（30 分钟）

**在哪做**：Linux VM（要 root；containerd 已在跑，见 Session 1-4）

**目标**：用 `runsc` 跑起容器，进容器跑 `dmesg` 看到 gVisor 的启动日志。

**前置自检**：

```bash
sudo systemctl status containerd --no-pager | head -3   # active (running)
uname -m   # aarch64（下对应架构的 runsc）
```

**动手**（安装步骤以 [gVisor 官方安装文档](https://gvisor.dev/docs/user_guide/install/) 为准，版本/源地址打开时核对）：

1. 装 runsc（二进制或 apt 源，文档二选一），装完：

```bash
runsc --version   # 预期：打印版本号
```

2. 用 runsc 当 runtime 跑 busybox：

```bash
sudo ctr image pull docker.io/library/busybox:latest
sudo ctr run --rm --runtime io.containerd.runsc.v1 docker.io/library/busybox:latest g1 /bin/sh -c "dmesg | head -5; uname -r"
```

预期输出：dmesg 里看到 gVisor 的启动日志（`gVisor` 字样）；`uname -r` 显示的是 gVisor 假内核的版本，不是宿主机的。

3. 对照 runc 跑同一个命令，感受区别：

```bash
sudo ctr run --rm docker.io/library/busybox:latest r1 /bin/sh -c "uname -r"
```

预期：这次显示的是**宿主机**内核版本。

**刚才发生了什么**：runc 的容器和宿主机共享同一个内核（隔离靠 namespace + seccomp）；gVisor 在中间插了一个用户态内核（Sentry），容器的 syscall 先被它拦截处理，只有少数才透给宿主机。所以容器里 `uname` 看到的是假内核。

**验证**：一句话说出 gVisor 和 runc 隔离边界的区别（runc：共享宿主机内核；gVisor：用户态内核拦截 syscall）。

**自查清单**：

```bash
runsc --version
sudo ctr run --rm --runtime io.containerd.runsc.v1 docker.io/library/busybox:latest g2 /bin/sh -c "uname -r"
```

**常见坑**：（待实机补充）

**下一步**：→ Session 3-4（Kata：K8s 里跑轻量 VM）

---

## Session 3-4｜Kata：k3s 里跑 Kata pod（35 分钟）

**在哪做**：Linux VM（要 root，`/dev/kvm` 存在）

**目标**：k3s 单节点上跑起一个 `runtimeClassName: kata` 的 pod，`uname -r` 显示 guest 内核。

**前置自检**：

```bash
ls /dev/kvm && echo kvm-ok
free -g | head -2   # 可用内存 ≥ 4G（k3s + kata 比较吃内存，不够就先跳过本节）
```

**动手**：

1. 装 k3s 单节点：

```bash
curl -sfL https://get.k3s.io | sh -
sudo kubectl get nodes   # 预期：STATUS Ready
```

2. 装 kata-deploy：按 [Kata 官方 quick-start](https://github.com/kata-containers/kata-containers/blob/main/docs/quick-start-guide.md) 走（yaml 路径/版本以文档为准），装完检查：

```bash
sudo kubectl get runtimeclass   # 预期：看到 kata
```

3. 跑 Kata pod：

```bash
cat > /tmp/pod-kata.yaml <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: kata-demo
spec:
  runtimeClassName: kata
  containers:
  - name: c
    image: busybox
    command: ["sh", "-c", "sleep 3600"]
EOF
sudo kubectl apply -f /tmp/pod-kata.yaml
sudo kubectl wait --for=condition=Ready pod/kata-demo --timeout=180s
sudo kubectl exec kata-demo -- uname -r
```

预期：`uname -r` 显示的是 **guest 内核**版本（和宿主机 `uname -r` 不一样）——pod 里跑的是一台轻量 VM。

4. 对照：把 `runtimeClassName` 删掉再 apply 一个普通 pod，`uname -r` 显示宿主机内核。

**刚才发生了什么**：RuntimeClass 是"给 Pod 选 runtime"的开关；`kata` 这个 runtime 背后是轻量 VM（QEMU + KVM），每个 pod 独占一个 guest 内核。K8s 调度完全不用改，只换 runtime。

**验证**：说出 Kata 和 gVisor 隔离方式的本质区别（一句话：Kata 是硬件虚拟化真内核，gVisor 是用户态假内核）。

**自查清单**：

```bash
sudo kubectl get runtimeclass
sudo kubectl get pods
sudo kubectl exec kata-demo -- uname -r
```

**本节可暂缓**：k3s + kata 是 Phase 3 里最重的一节，拉镜像慢就先放放，**不阻塞 Session 3-5**。

**常见坑**：（待实机补充）

**下一步**：→ Session 3-5（bench.go：三方案对比评测）

---

## Session 3-5｜bench.go：三方案对比评测（40 分钟）

**在哪做**：Linux VM（要 root）

**目标**：输出 `RESULTS.md`：三方案启动耗时中位数 + 内存开销 + 对比表 + 3 条结论。

**前置自检**：

```bash
go version   # 预期：go1.22+
ls ~/workspace/sandbox-study/phase-3-sandbox-compare/impl/
```

**动手**：

1. 准备 runc 的 bundle（复用 Phase 1 的 rootfs，手写一个最小 config）：

```bash
mkdir -p /tmp/bench/bundle/rootfs && cd /tmp/bench/bundle
# 把你 Phase 1 的 rootfs（busybox 那棵）拷过来，或随便一个有 /bin/sh 的 rootfs
runc spec   # 生成 config.json
# 把 config.json 的 process.args 改成 ["/bin/sh", "-c", "exit 0"]，terminal 改 false
```

2. 把下面的 `bench.go` 存进 `phase-3-sandbox-compare/impl/bench.go`：

```go
// bench.go：三方案启动耗时对比（runc / runsc / firecracker）。
// 用法：sudo go run bench.go [firecracker_ms]
// firecracker_ms：按第 3 步手动测 5 次取中位数后填入。
// runc/runsc 部分自动跑；firecracker 部分手动计时（API + 等 ssh 就绪不好自动化）。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

func median(ds []float64) float64 {
	sort.Float64s(ds)
	return ds[len(ds)/2]
}

func memAvailMB() float64 {
	b, _ := os.ReadFile("/proc/meminfo")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "MemAvailable:") {
			f := strings.Fields(l)
			kb, _ := strconv.ParseFloat(f[1], 64)
			return kb / 1024
		}
	}
	return 0
}

// timeCmd 跑外部命令，返回耗时毫秒；失败直接退出（计时就别吞错了）。
func timeCmd(dir, name string, args ...string) float64 {
	t := time.Now()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "命令失败 %s %v: %v\n%s\n", name, args, err, out)
		os.Exit(1)
	}
	return float64(time.Since(t).Milliseconds())
}

func main() {
	const bundle = "/tmp/bench/bundle"
	fcMs := 0.0
	if len(os.Args) > 1 {
		fcMs, _ = strconv.ParseFloat(os.Args[1], 64)
	}

	// runc：前台 run 一个 exit 0 的容器，测 create+start+exit 全程，5 次取中位数。
	runcTs := []float64{}
	for i := 0; i < 5; i++ {
		runcTs = append(runcTs, timeCmd(bundle, "runc", "run", fmt.Sprintf("bench-%d", i)))
	}

	// runsc：经 ctr 跑 /bin/true，5 次取中位数（先 pull 好镜像）。
	runscTs := []float64{}
	for i := 0; i < 5; i++ {
		runscTs = append(runscTs, timeCmd("", "ctr", "run", "--rm",
			"--runtime", "io.containerd.runsc.v1",
			"docker.io/library/busybox:latest", fmt.Sprintf("gbench-%d", i), "/bin/true"))
	}

	// 内存开销：起 1 个 runc 容器，看 MemAvailable 差值（容器方案≈0，firecracker≈guest 内存）。
	m0 := memAvailMB()
	timeCmd(bundle, "runc", "run", "bench-mem")
	m1 := memAvailMB()

	fmt.Printf(`## 启动耗时（5 次中位数）

| 方案 | 中位数 |
|---|---|
| runc | %.0f ms |
| runsc (gVisor) | %.0f ms |
| firecracker | %.0f ms（手动填入） |

## 内存开销（单实例，MemAvailable 差值）

| 方案 | 差值 |
|---|---|
| runc 起一个容器 | %.1f MB |
| firecracker | ≈ guest 内存（512MB 配置 → 约 550MB，含 VMM 开销） |

## 对比表

| | 隔离边界 | 启动速度 | 内存开销 | 适用场景 |
|---|---|---|---|---|
| runc | 进程级（namespace+seccomp，共享内核） | 最快 | ≈0 | 普通容器 |
| gVisor | syscall 拦截（用户态内核） | 中 | 小 | 不可信代码、多租户容器 |
| firecracker | 硬件虚拟化（真内核） | 慢（百 ms 级） | 大（guest 内存） | 强隔离的 serverless/沙盒 |

## 结论（按你的实测数据填）

1. 启动速度：___ 最快（约 ___ms），适合 ___。
2. 隔离强度：___ 最强（___），代价是 ___。
3. Agent 沙盒选型：如果 ___ 选 ___，因为 ___。
`, median(runcTs), median(runscTs), fcMs, m0-m1)
}
```

3. 手动测 firecracker 启动 5 次（从 `InstanceStart` 到 ssh 就绪），取中位数：

```bash
for i in 1 2 3 4 5; do
  s=$(date +%s%N)
  curl -s -X PUT --unix-socket /tmp/fc.socket http://localhost/actions \
    --data '{"action_type": "InstanceStart"}'
  until ssh -o StrictHostKeyChecking=no -o ConnectTimeout=1 root@172.16.0.2 true 2>/dev/null; do sleep 0.2; done
  e=$(date +%s%N); echo "第 $i 次：$(( (e-s)/1000000 ))ms"
  # 停掉 microVM 再测下一次（按 getting-started 的关机方式）
done
```

（socket 路径和关机方式按你 3-2 的实际来；上面是模板。）

4. 跑 bench，输出 RESULTS.md：

```bash
cd ~/workspace/sandbox-study/phase-3-sandbox-compare/impl
sudo ctr image pull docker.io/library/busybox:latest   # runsc 计时要用
sudo go run bench.go <firecracker中位数> > ../RESULTS.md
cat ../RESULTS.md
```

预期：`RESULTS.md` 里有三组数字 + 对比表。3 条结论按你的实测数据填完。

**刚才发生了什么**：runc/runsc 的计时是"调外部命令看墙钟"，简单可靠；firecracker 的启动涉及 API + guest 内核 + ssh 三段，不好自动化，手动 5 次取中位数更诚实。内存开销读 `/proc/meminfo` 的 `MemAvailable` 差值——容器方案这个数≈0（共享内核），这本身就是一条结论。

**验证**：说出"为什么 firecracker 不自动化计时"（三段式启动，ssh 就绪的判定条件不稳定，手动更诚实）。

**自查清单**：

```bash
ls -lh ../RESULTS.md   # 非空
grep -c "ms" ../RESULTS.md
```

**常见坑**：（待实机补充）

**下一步**：→ 📦 项目收尾（RESULTS.md 定稿）→ Phase 4

---

## 📦 项目：sandbox-compare

`impl/bench.go` + `RESULTS.md`（对比表 + 3 条结论）+ 本 README 即项目文档。

**注意**：`bench.go` 的正文在 Session 3-5 第 2 步，跑通后把文件也提交到 `impl/`。

## Phase 3 出口验收

1. 独立起 VM、打快照、恢复（3-1）。
2. Firecracker microVM 跑通，gVisor 和 Kata 各跑通一次（3-2/3-3/3-4）。
3. `RESULTS.md` 有对比表和 3 条结论（3-5）。
