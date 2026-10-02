package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// run = pull + unpack + runc run，一条龙。
//
// 职责边界很清楚：跑容器这件事**不是**我们实现的，是 runc 实现的。
// 我们只负责准备好 bundle（rootfs + config.json），然后把它交给 runc。
func run(ref string) error {
	// 建 namespace、挂载都需要 root。与其让 runc 报一堆 permission denied，
	// 不如在这里就说清楚该怎么办。
	if os.Geteuid() != 0 {
		return fmt.Errorf("run 需要 root（要建 namespace 和挂载），请用: sudo %s run %s",
			filepath.Base(os.Args[0]), ref)
	}
	if _, err := exec.LookPath("runc"); err != nil {
		return fmt.Errorf("找不到 runc（run 只能在 Linux 上跑）: %w", err)
	}

	if err := pull(ref); err != nil {
		return err
	}
	if err := unpack(ref); err != nil {
		return err
	}

	bundleDir := filepath.Join("work", "output", "bundle")
	id := containerID(ref)

	// 收尾：runc run 正常结束会自己清掉状态，但如果中途出错（比如命令不存在），
	// 容器会停在 "stopped" 状态，runc list 里留个垃圾 —— 所以无条件 delete 一次。
	defer func() {
		_ = exec.Command("runc", "delete", "--force", id).Run()
	}()

	cmd := exec.Command("runc", "run", id)
	cmd.Dir = bundleDir // runc 在哪个目录找 config.json 和 rootfs：bundle 的根
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行: runc run %s  (bundle=%s)\n", id, bundleDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("runc run 失败: %w", err)
	}
	fmt.Println("容器已退出")
	return nil
}

// containerID 给容器起个名字：busybox:latest -> mini-oci-busybox
func containerID(ref string) string {
	r := newRegistry(ref)
	name := r.repo
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return "mini-oci-" + name
}
