# Phase 3｜虚拟化与轻量沙盒（3–4 周）

> 环境：Linux VM（ARM64，`/dev/kvm` 存在，要 root）｜ 前置：Phase 2 完成
>
> **目标**：亲手跑通 KVM / Firecracker / gVisor / Kata，输出一张"隔离边界 / 启动速度 / 内存开销 / 适用场景"对比表。

## 环境前置（2026-10-06 实机修正，先看这个）

| 检查 | x86_64（WSL2，本次采用） | aarch64（Mac + Lima） |
|---|---|---|
| 能不能跑本节 | ✅ 实测 18 秒起到 `login:`、无 RCU stall、时钟正常 | ❌ **过不了 guest 实测**：Apple VZ 的嵌套虚拟化时钟错乱（guest 时间快 2.3 倍）→ 卡死在 initramfs |
| 云镜像 | `jammy-server-cloudimg-amd64.img` | `...-arm64.img` |
| 固件 | 走 BIOS，**不需要 UEFI** | **UEFI-only**：要 `qemu-efi-aarch64` + `--boot uefi` |
| Firecracker 包 | `firecracker-…-x86_64.tgz` | `…-aarch64.tgz` |

**四条判据**（`ls /dev/kvm` 只是必要条件，不是充分条件）：能到 `login:`、`dmesg` 无 `rcu.*stall`、guest 时钟与宿主一致、guest `uname -r` ≠ 宿主。完整证据见 [mistakes/错题本.md](mistakes/错题本.md)。

**兜底命令**（libvirt 抽风时用它，也更能体现"KVM 是内核能力、QEMU 是用户态程序"）：

```bash
qemu-system-x86_64 -M q35 -accel kvm -cpu host -smp 2 -m 2048 \
  -drive if=virtio,file=/tmp/jammy-server-cloudimg-amd64.img,format=qcow2 \
  -drive if=virtio,file=/tmp/seed.iso,format=raw,readonly=on \
  -netdev user,id=n0,hostfwd=tcp::2222-:22 -device virtio-net-pci,netdev=n0 \
  -nographic
```

（把 `-accel kvm` 换成 `-accel tcg`，就亲身感受得到"有没有 KVM 加速"的差别。）

---

## 入口自测（15 分钟，先做这个）

做 `docs/入口自测.md` 的 Phase 3 五道题。判定：全对可压缩/跳过已掌握的 session；错 3 题以上按计划完整学。

## 进度

- [x] Session 3-1｜KVM：起一台 VM + 快照（WSL2/x86_64；起 VM 18 秒、快照恢复验证通过）
- [x] Session 3-2｜Firecracker：第一个 microVM（WSL2/x86_64；microVM 点火成功，guest 内核 6.18.51+ ≠ 宿主 6.18.40.1-microsoft-standard-WSL2，tap0 172.16.0.1/30 + NAT 出网验证通过）
- [x] Session 3-3｜gVisor：runsc 跑容器（WSL2/x86_64；Docker 29.8.2 + runsc release-20260928.0，容器内 uname 看到 4.19.0-gvisor 假内核 + Starting gVisor，runc 对照看到宿主内核 6.18.40.1）
- [x] Session 3-4｜Kata：k3s 里跑 Kata pod（WSL2/x86_64；k3s v1.36.5 + kata 4.2.0，kata pod 内 uname 看到 guest 内核 6.18.35，普通 pod 看到宿主内核）
- [ ] Session 3-5｜bench.go：三方案对比评测
- [ ] RESULTS.md 对比表 + 3 条结论

---

## Session 3-1｜KVM：起一台 ARM64 VM + 快照（40 分钟）

**在哪做**：Linux VM（要 root，`/dev/kvm` 存在）

**目标**：用 cloud image 起一台 VM（x86 用 amd64 / ARM64 用 arm64），打快照、恢复，验证回到快照点。

**前置自检**：

```bash
ls -l /dev/kvm && echo kvm-ok
uname -m   # x86_64（WSL）/ aarch64（Mac Lima）
```

⚠️ 先读上面「环境前置」：**Apple Silicon 的 Lima 过不了 guest 实测**，本节在 WSL2 上做。

**动手**：

1. 装包：

```bash
# x86_64：
sudo apt update && sudo apt install -y qemu-system-x86 qemu-utils libvirt-daemon-system virtinst genisoimage cpu-checker
virsh --version   # 预期：有版本号输出
sudo kvm-ok       # 预期：KVM acceleration can be used
# ARM64 把 qemu-system-x86 换成 qemu-system-arm + qemu-efi-aarch64
```

2. 下 Ubuntu cloud image（**按架构选**，x86 用 amd64）：

```bash
cd /tmp && wget -c --show-progress \
  https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img \
  -O demo.qcow2
qemu-img info demo.qcow2 | grep "virtual size"   # 预期：virtual size: 2.2G 之类
sudo cp demo.qcow2 /var/lib/libvirt/images/demo.qcow2
```

（ARM64 机器把 `-amd64` 换成 `-arm64`，并在第 4 步加 `--boot uefi`——ARM64 云镜像是 **UEFI-only**，少这个参数会停在 `no bootable device`。）

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

4. 起 VM（`--import` 直接用现成磁盘，不走安装）。**先确认默认网络是 active**——libvirt 的 default 网络默认是 inactive，不启会报 `Network not found`：

```bash
sudo virsh net-list --all
# State 是 inactive 就先启：
sudo virsh net-start default && sudo virsh net-autostart default
```

```bash
sudo virt-install --name demo --ram 2048 --vcpus 2 \
  --disk path=/var/lib/libvirt/images/demo.qcow2 \
  --disk path=/tmp/seed.iso,device=cdrom \
  --os-variant ubuntu22.04 --network network=default \
  --graphics none --console pty,target_type=serial --import
```

预期：约 20–40 秒后出现 `demo login:`。用 **`ubuntu`** / `demo123` 登录（⚠️ **Ubuntu 云镜像的默认用户是 `ubuntu`，不是 `demo`**）。退出 console 按 `Ctrl+]`。

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

**常见坑**：

- **`no bootable device` / 卡在 `Booting from Hard Disk...`（仅 ARM64）**：ARM64 云镜像 **UEFI-only**，要装 `qemu-efi-aarch64` 并加 `--boot uefi`（x86 走 BIOS，不需要）。
- **固件层 `Synchronous Exception`（ARM64 + libvirt）**：libvirt 自动选的是 `AAVMF_CODE.secboot.fd`（带安全启动），加载 `shimaa64.efi` 时在嵌套虚拟化下崩。显式指定非 secboot 固件：`--boot loader=/usr/share/AAVMF/AAVMF_CODE.fd,loader_ro=yes,loader_type=pflash,nvram_template=/usr/share/AAVMF/AAVMF_VARS.fd`。
- **`Network not found: no network with matching name 'default'`**：`sudo virsh net-start default && sudo virsh net-autostart default`。
- **`demo / demo123` 登录不上**：默认用户是 **`ubuntu`**（实机证据：guest 日志 `ci-info: no authorized SSH keys fingerprints found for user ubuntu.`）。
- **guest 卡在 initramfs、60 秒后爆 `rcu_sched detected stalls`**：这台机器的**嵌套虚拟化时钟不可信**（guest 时间比墙钟快 2.3 倍）。换 WSL2/真 KVM 环境——见 [mistakes/错题本.md](mistakes/错题本.md)。

**下一步**：→ Session 3-2（Firecracker：比这台 VM 轻 100 倍的 microVM）

---

## Session 3-2｜Firecracker：第一个 microVM（40 分钟）

**在哪做**：WSL2（`ssh PCGaming`，用户 `kms`；要 root 密码，sudo 的命令你亲手敲）

**目标**：跑通官方 getting-started，ssh 进 microVM，`uname -r` 和宿主机不一样。

**前置自检**：

```bash
ls /dev/kvm && echo kvm-ok
uname -m   # 预期：x86_64（下 x86_64 的二进制和 kernel）
mkdir -p ~/fc && cd ~/fc && pwd   # 本节工作目录
```

**动手**（跟着[官方 getting-started](https://github.com/firecracker-microvm/firecracker/blob/main/docs/getting-started.md) 走，**kernel/rootfs 链接以文档为准**，不要背）：

1. 下 Firecracker 二进制（v1.17.0，x86_64）：

```bash
cd ~/fc
wget https://github.com/firecracker-microvm/firecracker/releases/download/v1.17.0/firecracker-v1.17.0-x86_64.tgz
tar xzf firecracker-v1.17.0-x86_64.tgz
./release-v1.17.0-x86_64/firecracker-v1.17.0-x86_64 --version   # 预期：打印 v1.17.0
```

2. 下 kernel + rootfs：按 getting-started 文档的 **x86_64** 链接拿 kernel 和 `ubuntu-22.04.ext4`。
   ⚠️ **kernel 必须是未压缩的 vmlinux**——Firecracker 不认压缩内核，别拿宿主 `/boot/vmlinuz` 顶替（那是 gzip 包过的）。

```bash
file vmlinux   # 预期：ELF 64-bit LSB executable，不要看到 "gzip compressed"
ls -lh ubuntu-22.04.ext4 vmlinux
```

3. 配 tap 网络（宿主机侧；tap + NAT 要自己写，WSL 本地做，没有锁门风险）：

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

**实机记录**（2026-10-06，WSL2/x86_64，Firecracker v1.17.0）：

- `uname -r`：guest `6.18.51+` vs 宿主 `6.18.40.1-microsoft-standard-WSL2`——两个内核，microVM 跑的是自己的内核。
- 网络：宿主 `tap0` = `172.16.0.1/30`（UP），guest `eth0` = `172.16.0.2/30`（`boot_args` 里 `ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off` 配的）；guest 内 `ping 8.8.8.8` 0 丢包，宿主 NAT（`eth0` 出口 MASQUERADE）生效。
- 配置：`machine-config` = 2 vCPU / 512 MiB；API 顺序 `boot-source` → `drives` → `network-interfaces` → `machine-config` → `actions`（`InstanceStart`）。

**常见坑**：见 `mistakes/错题本.md` 第 6、7、8 条（`curl && echo OK` 不可信、v1.17 `drives` body 缺 `drive_id`、kernel 文件名别脑补）。

**下一步**：→ Session 3-3（gVisor：不走硬件虚拟化的另一条路）

---

## Session 3-3｜gVisor：runsc 跑容器（30 分钟）

**在哪做**：WSL2（`ssh PCGaming`；装 docker 那一步要 sudo 密码，之后直连可跑）

**目标**：用 `runsc` 跑起容器，容器里 `uname -r` 看到 gVisor 的假内核版本，`dmesg` 看到 gVisor 启动日志；对照 runc 看到宿主机内核。

**前置自检**：

```bash
docker --version   # 29.x
runsc --version    # release-2026xxxx.x
uname -m           # x86_64
```

**动手**（安装步骤以 [gVisor 官方安装文档](https://gvisor.dev/docs/user_guide/install/) 为准，版本/源地址打开时核对）：

1. 装 docker-ce（Docker 官方 apt 源）：

```bash
sudo apt-get update && sudo apt-get install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update && sudo apt-get install -y docker-ce docker-ce-cli containerd.io
sudo usermod -aG docker kms   # 之后 docker 命令免 sudo（新登录生效）
```

2. 装 runsc（⚠️ `release/latest` 现在**只发 `gvisor.tar.bz2`**，单独下 `runsc` 二进制会 404）：

```bash
cd /tmp && curl -fsSL https://storage.googleapis.com/gvisor/releases/release/latest/x86_64/gvisor.tar.bz2 -o gvisor.tar.bz2 && curl -fsSL https://storage.googleapis.com/gvisor/releases/release/latest/x86_64/gvisor.tar.bz2.sha512 -o gvisor.tar.bz2.sha512 && sha512sum -c gvisor.tar.bz2.sha512
mkdir -p gvisor && tar -xjf gvisor.tar.bz2 -C gvisor
sudo install -m 0755 gvisor/runsc /usr/local/bin/runsc
sudo install -m 0755 gvisor/containerd-shim-runsc-v1 /usr/local/bin/containerd-shim-runsc-v1
sudo cp -r gvisor/gvisor-bin /usr/local/bin/
sudo /usr/local/bin/runsc install && sudo systemctl restart docker   # 注册 runsc 为 docker runtime
runsc --version   # 预期：打印版本号
```

3. 拉镜像、跑 gVisor 容器（⚠️ 这台机器直连 Docker Hub 会 EOF，改走 DaoCloud 镜像 `docker.m.daocloud.io`）：

```bash
docker pull docker.m.daocloud.io/library/busybox:latest
docker run --rm --runtime=runsc docker.m.daocloud.io/library/busybox:latest uname -r
# 预期：4.19.0-gvisor（gVisor 的假内核，不是宿主机的）
docker run --rm --runtime=runsc docker.m.daocloud.io/library/busybox:latest dmesg | head -3
# 预期：[   0.000000] Starting gVisor...
```

4. 对照 runc 跑同一个镜像：

```bash
docker run --rm docker.m.daocloud.io/library/busybox:latest uname -r
# 预期：宿主机内核 6.18.40.1-microsoft-standard-WSL2
```

**刚才发生了什么**：runc 的容器和宿主机共享同一个内核（隔离靠 namespace + seccomp）；gVisor 在中间插了一个用户态内核（Sentry），容器的 syscall 先被它拦截处理，只有少数才透给宿主机。所以容器里 `uname` 看到的是假内核（`4.19.0-gvisor`），`dmesg` 看到的是 Sentry 的启动日志（`Starting gVisor...` / `Feeding the init monster...`）。

**验证**：一句话说出 gVisor 和 runc 隔离边界的区别（runc：共享宿主机内核；gVisor：用户态内核拦截 syscall）。

**自查清单**：

```bash
docker run --rm --runtime=runsc docker.m.daocloud.io/library/busybox:latest uname -r   # 4.19.0-gvisor
docker run --rm docker.m.daocloud.io/library/busybox:latest uname -r                    # 宿主机内核
```

**实机记录**（2026-10-06 深夜，WSL2/x86_64，Docker 29.8.2，runsc release-20260928.0）：
- runsc：`uname -r` = `4.19.0-gvisor`；`dmesg` 首行 `[   0.000000] Starting gVisor...`
- runc：`uname -r` = `6.18.40.1-microsoft-standard-WSL2`
- 镜像走 `docker.m.daocloud.io`（Docker Hub 直连被 EOF，见错题本第 10 条）

**常见坑**：见 `mistakes/错题本.md` 第 9、10 条（latest 不再单独发 runsc 二进制、Docker Hub 直连 EOF 改走镜像）。

**下一步**：→ Session 3-4（Kata：K8s 里跑轻量 VM）

---

## Session 3-4｜Kata：k3s 里跑 Kata pod（35 分钟）

**在哪做**：WSL2（`ssh PCGaming`；装 k3s 那一步要 sudo 密码，之后直连可跑）

**目标**：k3s 单节点上跑起一个 `runtimeClassName: kata-qemu-runtime-rs` 的 pod，`uname -r` 显示 guest 内核。

**前置自检**：

```bash
ls /dev/kvm && echo kvm-ok
free -g | head -2   # 可用内存 ≥ 4G
```

**动手**：

1. 装 k3s 单节点（⚠️ k3s 自带的 kubectl 默认读 `/etc/rancher/k3s/k3s.yaml`，拷一份到 `~/.kube/config` 并 `export KUBECONFIG=~/.kube/config`）：

```bash
curl -sfL https://get.k3s.io | sh -
mkdir -p ~/.kube && sudo cp /etc/rancher/k3s/k3s.yaml ~/.kube/config && sudo chown $(id -u):$(id -g) ~/.kube/config
export KUBECONFIG=~/.kube/config
kubectl get nodes   # 预期：STATUS Ready
```

2. 装 helm（免 sudo），用 kata-deploy Helm chart 装 Kata（⚠️ Kata 4.x 的 RuntimeClass 改名了，没有叫 `kata` 的了；k3s 必须 `--set k8sDistribution=k3s`）：

```bash
mkdir -p ~/bin && cd /tmp && curl -fsSL https://get.helm.sh/helm-v4.3.0-linux-amd64.tar.gz -o helm.tar.gz && tar xzf helm.tar.gz && install -m 0755 linux-amd64/helm ~/bin/helm
export PATH=$HOME/bin:$PATH
helm install kata-deploy "oci://ghcr.io/kata-containers/kata-deploy-charts/kata-deploy" --version "4.2.0" --namespace kata-system --create-namespace --set k8sDistribution=k3s
kubectl -n kata-system get pods   # 等 kata-deploy 装完（/opt/kata 落地，日志显示 installation completed successfully）
kubectl get runtimeclass | grep kata-qemu-runtime-rs   # 预期：看到它
```

3. 跑 Kata pod 和普通 pod 对照（pod 跑完即退出，用 `kubectl logs` 看输出；镜像用 quay.io，Docker Hub 直连不通）：

```bash
cat > /tmp/kata-pods.yaml <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: kata-demo
spec:
  runtimeClassName: kata-qemu-runtime-rs
  restartPolicy: Never
  containers:
  - name: c
    image: quay.io/libpod/ubuntu:latest
    command: ["uname", "-r"]
---
apiVersion: v1
kind: Pod
metadata:
  name: runc-demo
spec:
  restartPolicy: Never
  containers:
  - name: c
    image: quay.io/libpod/ubuntu:latest
    command: ["uname", "-r"]
EOF
kubectl apply -f /tmp/kata-pods.yaml
kubectl wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=300s pod/kata-demo
kubectl wait --for=jsonpath='{.status.phase}'=Succeeded --timeout=300s pod/runc-demo
kubectl logs kata-demo   # 预期：guest 内核版本，和宿主机不一样
kubectl logs runc-demo   # 预期：宿主机内核
```

**刚才发生了什么**：RuntimeClass 是"给 Pod 选 runtime"的开关；`kata-qemu-runtime-rs` 这个 runtime 背后是轻量 VM（QEMU + KVM），每个 pod 独占一个 guest 内核。K8s 的调度、yaml 写法完全不用改，只换一行 `runtimeClassName`。

**验证**：说出 Kata 和 gVisor 隔离方式的本质区别（一句话：Kata 是硬件虚拟化真内核，gVisor 是用户态假内核）。

**自查清单**：

```bash
kubectl get runtimeclass | grep kata
kubectl logs kata-demo   # guest 内核
kubectl logs runc-demo   # 宿主机内核
```

**实机记录**（2026-10-07 凌晨，WSL2/x86_64，k3s v1.36.5，kata 4.2.0）：
- kata pod（`kata-qemu-runtime-rs`）：`uname -r` = `6.18.35`（guest 内核）
- 普通 pod：`uname -r` = `6.18.40.1-microsoft-standard-WSL2`（宿主内核）

**常见坑**：见 `mistakes/错题本.md` 第 11、12、13 条（老 kata-deploy.yaml 404、k3s 要 `--set k8sDistribution=k3s`、k3s kubectl 不认 `~/.kube/config`）。

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
