package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// imageConfig 是 config blob 里我们关心的那部分。
// 注意：字段名必须和 JSON 里的 key 完全对应（大写开头的导出字段 + tag）。
type imageConfig struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		Env        []string `json:"Env"`
		Entrypoint []string `json:"Entrypoint"`
		Cmd        []string `json:"Cmd"`
	} `json:"config"`
	RootFS struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

// unpack：把本地 layout 变成 runtime bundle（rootfs/ + config.json）。
// 全程只读磁盘，不联网 —— 这就是 OCI 分层的价值：pull 和 unpack 可以分开做。
func unpack(ref string) error {
	r := newRegistry(ref)
	layoutDir := outputDir(r)
	bundleDir := filepath.Join("work", "output", "bundle")
	rootfsDir := filepath.Join(bundleDir, "rootfs")
	fmt.Printf("layout: %s\n", layoutDir)
	fmt.Printf("bundle: %s\n", bundleDir)

	// 1. 从本地 layout 里读出三件套。
	//    注意：虽然文件在磁盘上，我们仍然按"地址"去读（blobs/sha256/<hex>），
	//    而不是记"哪个文件是 config" —— 内容寻址在本地也一样成立。
	var idx imageIndex
	if err := readJSON(filepath.Join(layoutDir, "index.json"), &idx); err != nil {
		return err
	}
	if len(idx.Manifests) != 1 {
		return fmt.Errorf("本地 index 应该有 1 个条目，实际 %d（是不是 pull 没跑完？）", len(idx.Manifests))
	}
	mraw, err := readBlob(layoutDir, idx.Manifests[0].Digest)
	if err != nil {
		return err
	}
	var m imageManifest
	if err := json.Unmarshal(mraw, &m); err != nil {
		return fmt.Errorf("解析 manifest 失败: %w", err)
	}
	craw, err := readBlob(layoutDir, m.Config.Digest)
	if err != nil {
		return err
	}
	var cfg imageConfig
	if err := json.Unmarshal(craw, &cfg); err != nil {
		return fmt.Errorf("解析 image config 失败: %w", err)
	}
	fmt.Printf("镜像平台: %s/%s\n", cfg.OS, cfg.Architecture)

	// layers 和 diff_ids 靠**下标**一一对应，这是 OCI 里没有显式关联的一处约定
	if len(m.Layers) != len(cfg.RootFS.DiffIDs) {
		return fmt.Errorf("layers(%d) 和 diff_ids(%d) 数量不一致", len(m.Layers), len(cfg.RootFS.DiffIDs))
	}

	// 2. 逐层解包（从空 rootfs 开始，保证可重复执行）
	if err := os.RemoveAll(rootfsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(rootfsDir, 0o755); err != nil {
		return err
	}
	for i, layer := range m.Layers {
		n, err := applyLayer(layoutDir, rootfsDir, layer, cfg.RootFS.DiffIDs[i])
		if err != nil {
			return fmt.Errorf("解包 layer %d 失败: %w", i, err)
		}
		fmt.Printf("layer %d: %s 解出 %d 个条目，diff_id 校验通过\n", i, layer.Digest, n)
	}

	// 3. 写 runtime bundle 的 config.json（OCI runtime-spec）
	spec := makeRuntimeSpec(cfg)
	fmt.Printf("容器默认命令: %v\n", spec.Process.Args)
	if err := writeJSONFile(filepath.Join(bundleDir, "config.json"), spec); err != nil {
		return err
	}
	fmt.Printf("完成: %s\n", bundleDir)
	return nil
}

// applyLayer 解压一层并**顺带校验 diff_id**。
//
// diff_id 是"解压后 tar"的 sha256（manifest 里那个 layer digest 是"压缩后"的），
// 所以校验必须发生在解压之后 —— 用 TeeReader 让数据只过一遍。
func applyLayer(layoutDir, rootfsDir string, l descriptor, wantDiffID string) (int, error) {
	p, err := blobPath(layoutDir, l.Digest)
	if err != nil {
		return 0, err
	}
	f, err := os.Open(p)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var src io.Reader = f
	if strings.HasSuffix(l.MediaType, "+gzip") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return 0, err
		}
		defer gz.Close() // 关闭时会校验 gzip 的 CRC
		src = gz
	}

	h := sha256.New()
	tr := tar.NewReader(io.TeeReader(src, h))

	n := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return n, err
		}
		if err := extractEntry(rootfsDir, hdr, tr); err != nil {
			return n, fmt.Errorf("%s: %w", hdr.Name, err)
		}
		n++
	}
	// tar.Reader 通常停在结束标记处，解压流可能还有尾巴（padding）没读。
	// diff_id 是整条解压流的 digest，所以要把剩下的字节也喂给 hash。
	if _, err := io.Copy(h, src); err != nil {
		return n, err
	}

	if got := "sha256:" + hex.EncodeToString(h.Sum(nil)); got != wantDiffID {
		return n, fmt.Errorf("diff_id 不符：声明 %s，实际 %s", wantDiffID, got)
	}
	return n, nil
}

// extractEntry 把一个 tar 条目落到 rootfs 里。
func extractEntry(rootfsDir string, hdr *tar.Header, tr io.Reader) error {
	base := filepath.Base(hdr.Name)

	// OCI whiteout：上一层"删除文件"的标记（多层镜像叠加时才出现）
	if strings.HasPrefix(base, ".wh.") {
		dir := filepath.Dir(hdr.Name)
		if base == ".wh..wh..opq" {
			// opaque：这一层把这个目录里的内容全部清空
			p, err := safeJoin(rootfsDir, dir)
			if err != nil {
				return err
			}
			entries, err := os.ReadDir(p)
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			for _, e := range entries {
				if err := os.RemoveAll(filepath.Join(p, e.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		p, err := safeJoin(rootfsDir, filepath.Join(dir, strings.TrimPrefix(base, ".wh.")))
		if err != nil {
			return err
		}
		return os.RemoveAll(p)
	}

	target, err := safeJoin(rootfsDir, hdr.Name)
	if err != nil {
		return err
	}

	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(hdr.Mode)&0o777)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, tr)
		return err
	case tar.TypeSymlink:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.Symlink(hdr.Linkname, target)
	case tar.TypeLink:
		// 硬链接：目标必须已经解出来（tar 里通常排在被链接的文件之后）。
		// busybox 就是 1 个二进制 + 410 个硬链接，漏了这一步镜像基本就废了。
		src, err := safeJoin(rootfsDir, hdr.Linkname)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.Link(src, target)
	case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		// 设备节点要 mknod + CAP_MKNOD。runc 会自己挂 /dev，所以这里跳过。
		// 真实实现（docker/containerd）会老老实实 mknod 出来。
		return nil
	default:
		return nil
	}
}

// safeJoin 把 tar 里的路径安全地拼到 rootfs 下。
//
// tar 里的路径可能是 "../../etc/passwd" 这种恶意路径 —— 镜像是不受信任的输入。
// 技巧：先 filepath.Clean("/" + name) 把它归一化成绝对路径（多余的 .. 会被消掉），
// 再拼到 rootfs 下，最后用前缀检查双保险。
func safeJoin(root, name string) (string, error) {
	clean := filepath.Clean("/" + name)
	target := filepath.Join(root, clean)
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径越界: %q", name)
	}
	return target, nil
}

// readBlob 按 digest 从 layout 里读出一个 blob（用于 manifest/config 这种小文件）。
func readBlob(layoutDir, digest string) ([]byte, error) {
	p, err := blobPath(layoutDir, digest)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	// 本地读也顺手校验一次：磁盘上的内容也可能被改坏
	if got := digestOf(raw); got != digest {
		return nil, fmt.Errorf("%s 内容损坏：实际是 %s", digest, got)
	}
	return raw, nil
}

func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return nil
}

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
