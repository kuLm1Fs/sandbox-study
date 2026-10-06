# HANDOFF — sandbox-study

> 给接手的 AI 或人：**先读这一页**，再按里面的路径跳转，不要重读全部文档。
> 写于 2026-10-06 晚｜交接点：**Phase 3 Session 3-2（Firecracker）刚要开始**。

## 0. 一句话现状

Phase 1（mini-oci）✅ · Phase 2（mini-container v2.0）✅ · Phase 3 进行中：**3-1（KVM/QEMU/libvirt 起 VM + 快照）✅**，**3-2（Firecracker）Step 1 还没做**。

## 1. 这是什么

8 步学习闭环项目（`docs/学习方法.md`）：每个 phase 有 `知识点.md` / 讲义 `README.md` / `tests/TESTS.md` / `notes/` / `mistakes/错题本.md`，阶段结束写 `reports/`。总路线见 `docs/学习路线.md`。

**三条铁律**（沿用）：

1. **先做再看**——讲义里的"背景"不超过 5 行，动手先行；
2. **一条命令 + 预期输出**——凡是"打开文档找 XX"都改成可复制命令；
3. **错题本只收真实踩到的**——每条按"看到什么 → 为什么 → 怎么修"。

## 2. 三台机器（先搞清在哪敲命令，这是最容易出错的地方）

| 机器 | 怎么进 | 干什么 | 边界（实测） |
|---|---|---|---|
| **Mac**（主控） | 本地 | 写代码/文档、git 主战场 | — |
| **Lima VM `sandbox`**（在 Mac 里） | `limactl shell sandbox` | Phase 1/2 的 Linux 实验 | aarch64、8 GiB；⚠️ **嵌套 guest 跑不起来**（见 §5） |
| **PC 的 WSL2**（主机名 `PCGAMING`） | `ssh PCGaming`（ssh config → Tailscale `100.65.72.40`，用户 `kms`） | **Phase 3/4**：KVM / Firecracker / Kata / k8s | ✅ x86_64、20 CPU/15 GiB、`/dev/kvm` 可用、`networkingMode=mirrored` |

- **sudo 情况**：WSL 上 `kms` **需要密码** → AI 自己装包 / `virsh` 写操作都不行，**要交给用户敲**。Mac 上 `limactl`、`git` 免密。
- **git**：`origin` = `git@github.com:kuLm1Fs/sandbox-study.git`（**public**）。别提交密钥、公网 IP、token。提交风格：中文、首行 `Phase X Session Y：一句话`、正文列改动与验证。
- EC2 VPS（`34.212.64.201`，Phase 2 用过）**已停用**，不要再用。

## 3. 已完成（读这些就够，别重复劳动）

| 阶段 | 交付物 | 报告/文档 |
|---|---|---|
| Phase 1 | `phase-1-mini-oci/impl/`（pull/unpack/run，943 行）+ `tools/`（手搓版参照物，产物可与 impl 逐字节 diff） | `reports/phase-1-学习报告.md` |
| Phase 2 | `phase-2-container-v2/impl/`（main/seccomp/network，447 行：cgroup + seccomp + netns/veth/NAT） | `reports/phase-2-学习报告.md` |
| Phase 3-1 | WSL2 上 `virt-install` 起 VM、快照/恢复验证通过（18 秒到 login） | `phase-3-sandbox-compare/README.md` Session 3-1、`mistakes/错题本.md`（5 条） |

## 4. 交接点：Session 3-2（Firecracker）

**确切状态**：WSL 上 `~/fc` 目录还没建 → **Step 1（下 `firecracker v1.17.0` x86_64 二进制、验版本）未执行**。讲义在 `phase-3-sandbox-compare/README.md` 的 Session 3-2，"环境前置"里有四条判据和裸 QEMU 兜底命令。

**三步走**：

1. 二进制：`wget https://github.com/firecracker-microvm/firecracker/releases/download/v1.17.0/firecracker-v1.17.0-x86_64.tgz` → `--version`
2. kernel（**未压缩 vmlinux**）+ rootfs（`ubuntu-22.04.ext4`），链接以官方 getting-started 为准
3. 起 microVM：unix socket + `curl` 依次发 `boot-source` → `drives` → `network-interfaces` → `machine-config` → `actions(InstanceStart)`

**降级路径**（讲义也写了）：tap/ssh 卡住就先跳过——**只要串口看到启动日志、能敲命令就算过**。
**预判坑**：FC 不认压缩内核（别拿宿主 `/boot/vmlinuz` 顶替）；tap + NAT 要自己写（在本地 WSL 做，没有锁门风险，不像云 VPS）。

## 5. 已知陷阱（细节见 `phase-3-sandbox-compare/mistakes/错题本.md`，这里是索引）

- **Apple Silicon 的 Lima(VZ) 跑不了嵌套 guest**：`/dev/kvm` 在，但 guest 时钟快 2.3 倍 → initramfs 卡死 + RCU stall。**Phase 3 因此搬到 WSL2**。教训：`ls /dev/kvm` 只是必要条件。
- **别复用"跑过一次"的云镜像**：cloud-init 会固化网卡名 → 换到 libvirt 下拿不到 DHCP，卡 `wait-online (no limit)`。用"母盘 + `qemu-img create -b` 派生盘"。
- **验证环境要看实物证据**：四条判据 = 到 `login:` / 无 `rcu.*stall` / guest 时钟与宿主一致 / guest `uname -r` ≠ 宿主。
- **讲解顺序**：先概念后代码。Phase 2 曾因一次性丢大量 `syscall`/`unix.*` 代码导致"完全看不懂"，补救办法（`strace` 逐行对表）反而成了最好的材料——**默认先给那张表**。

## 6. WSL 上留着的现场（可复用 / 该清理）

- `~/sandbox-study/`（已 clone）
- libvirt：domain **`demo` 正在 running**；快照 `snap1`(22:01:19)、`snap2`(22:09:25)
- `/tmp/demo-amd64.img`(697M，**已被写过，只能当"脏盘"样本**)、`/tmp/seed.iso`、`/tmp/boot.log`（裸 QEMU 成功那次的日志）
- 干净母盘 `/var/lib/libvirt/images/jammy-master.qcow2`（派生用）
- 已装：`qemu-system-x86 qemu-utils libvirt-daemon-system virtinst genisoimage cpu-checker`；**Go 未装**（3-5 的 `bench.go` 需要）
- Mac 的 Lima 里：`/tmp/jammy-server-cloudimg-arm64.img`、`/tmp/seed.iso`（Phase 3 用不上，可删）

## 7. 接手后先跑这几条确认状态

```bash
cd ~/sandbox-study && git log --oneline -1 && git status -sb      # 期望 6a987bf、干净
ssh PCGaming 'ls ~/fc 2>/dev/null; virsh list | tail -2; virsh snapshot-list demo'
limactl list                                                      # Mac 上 sandbox 是否还在跑
```

## 8. 建议下一个 session 用什么

- 没有专用 skill。**若 3-2 出诡异问题**（FC 起不来、网络不通），用 `diagnose` 的流程：复现 → 最小化 → 假设 → 仪表 → 修 → 回归。
- 学习产物继续按八步闭环结构走（讲义 / 知识点 / tests / notes / mistakes / reports），**别另起一套**。
