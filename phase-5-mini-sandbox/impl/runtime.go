package main

// Runtime 是所有沙盒后端的合同。
// 上层（任务池）只认这四个方法，不认底下是 runc 还是 firecracker。
// W2 加 firecracker 后端时，上层一行不用改——
// 跟 4-2 一个道理：apiserver 不关心账本是 etcd 还是 sqlite。
type Runtime interface {
	// Create 建沙盒（不起）：runc 版 = docker create；
	// firecracker 版 = 配好 kernel/rootfs/网络，等 InstanceStart。
	Create(id, image string) error
	// Start 起沙盒。
	Start(id string) error
	// Stop 停沙盒。
	Stop(id string) error
	// Destroy 删沙盒，清理干净。
	Destroy(id string) error
}
