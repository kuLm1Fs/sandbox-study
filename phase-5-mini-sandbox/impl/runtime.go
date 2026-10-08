package main

// Runtime 是所有沙盒后端的合同。
// 上层（任务池）只认这五个方法，不认底下是 runc 还是 firecracker。
// W2 加 firecracker 后端时，上层一行不用改——
// 跟 4-2 一个道理：apiserver 不关心账本是 etcd 还是 sqlite。
// W3：合同长大，加 Exec（验收用）。加方法后，两个后端都要补实现，
// 少补一个编译器就报错——这就是接口的约束力。
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
	// Exec 在沙盒里跑命令，返回合并后的输出（验收用）。
	// runc 版 = docker exec，firecracker 版 = ssh。
	Exec(id string, cmd ...string) (string, error)
}
