package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Snapshotter 是可选能力接口：给 VM 拍快照、从快照恢复。
// 不是所有后端都会做快照（runc 就做不了），所以不塞进 Runtime，
// 单独成一个小接口——跟 io.Seeker 一个道理：不是所有 io.Reader 都能 Seek。
// 用的时候做类型断言：
//
//	s, ok := rt.(Snapshotter)
//	if !ok { /* 这个后端不会快照 */ }
type Snapshotter interface {
	// Snapshot 给运行中的 VM 拍快照（Full：全量内存+设备状态存文件）。
	// 注意：拍完 VM 会被暂停。
	Snapshot(id, snapPath, memPath string) error
	// Restore 从快照文件起一个新 VM 并唤醒运行。
	Restore(id, snapPath, memPath string) error
}

var _ Snapshotter = (*FirecrackerRuntime)(nil)

// Snapshot 调 snapshot/create API，把内存和设备状态存文件。
// 注意两点：① 必须先 PATCH /vm 暂停，Running 的 VM 直接拍会 400
// （"save/restore unavailable while running"，W4 实机踩到）；
// ② 拍完 VM 保持暂停，我们的流程是接着 Stop/Destroy，所以不用 resume。
func (f *FirecrackerRuntime) Snapshot(id, snapPath, memPath string) error {
	_ = id
	if err := f.api("PATCH", "/vm", `{"state":"Paused"}`); err != nil {
		return fmt.Errorf("暂停 VM 失败: %v", err)
	}
	return f.api("PUT", "/snapshot/create", fmt.Sprintf(
		`{"snapshot_type":"Full","snapshot_path":%q,"mem_file_path":%q}`, snapPath, memPath))
}

// Restore 起一个新 firecracker 进程，用 snapshot/load 把 VM 从快照唤醒
//（resume_vm=true：加载完直接跑），再轮询 ssh 等客户机就绪。
func (f *FirecrackerRuntime) Restore(id, snapPath, memPath string) error {
	_ = id
	os.Remove(f.Sock)
	f.proc = exec.Command(f.Bin, "--api-sock", f.Sock)
	if err := f.proc.Start(); err != nil {
		return fmt.Errorf("起 firecracker 失败: %v", err)
	}
	if err := f.waitSock(5 * time.Second); err != nil {
		return err
	}
	if err := f.api("PUT", "/snapshot/load", fmt.Sprintf(
		`{"snapshot_path":%q,"mem_file_path":%q,"resume_vm":true}`,
		snapPath, memPath)); err != nil {
		return err
	}
	return f.waitSSH()
}
