package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// pull 拉取镜像并写成 OCI layout：
//
//	<out>/oci-layout
//	<out>/index.json
//	<out>/blobs/sha256/<digest>
//
// 顺序不能乱：先写叶子（config/layers），再写 manifest（它引用叶子），
// 最后写 index.json（它引用 manifest）—— 因为父对象要写子对象的 digest 和大小。
func pull(ref string) error {
	r := newRegistry(ref)
	out := outputDir(r)
	fmt.Printf("镜像: %s  (repo=%s tag=%s)\n", ref, r.repo, r.tag)
	fmt.Printf("输出: %s\n", out)

	// 1. tag 指向的可能不是 manifest，而是多架构 index（路牌）
	raw, mediaType, err := r.fetchManifest(r.tag)
	if err != nil {
		return err
	}
	fmt.Printf("拿到: %s\n", mediaType)

	selected := "" // 如果是从 index 挑的，记下它声明的 digest
	if mediaType == mtIndex {
		var idx imageIndex
		if err := json.Unmarshal(raw, &idx); err != nil {
			return fmt.Errorf("解析 index 失败: %w", err)
		}
		selected, err = pickPlatform(idx, "linux", runtime.GOARCH)
		if err != nil {
			return err
		}
		fmt.Printf("index 里选中 linux/%s -> %s\n", runtime.GOARCH, selected)

		// 2. 用 digest 再拿一次（tag 可变，digest 不可变）
		raw, mediaType, err = r.fetchManifest(selected)
		if err != nil {
			return err
		}
	}
	if mediaType != mtManifest {
		return fmt.Errorf("期望 %s，却拿到 %s", mtManifest, mediaType)
	}

	var m imageManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("解析 manifest 失败: %w", err)
	}

	// 3. manifest 的地址由我们自己算（它是引用链的根）
	manifestDigest := digestOf(raw)
	if selected != "" && selected != manifestDigest {
		return fmt.Errorf("index 说 %s，实际字节算出来是 %s", selected, manifestDigest)
	}

	// 4. 先写叶子：config + layers
	fmt.Printf("config: %s (%d 字节)\n", m.Config.Digest, m.Config.Size)
	if err := writeRemoteBlob(r, out, m.Config); err != nil {
		return err
	}
	for i, l := range m.Layers {
		fmt.Printf("layer %d: %s (%d 字节)\n", i, l.Digest, l.Size)
		if err := writeRemoteBlob(r, out, l); err != nil {
			return err
		}
	}

	// 5. 再写 manifest blob（字节就是地址）
	md := descriptor{
		MediaType: mtManifest,
		Digest:    manifestDigest,
		Size:      int64(len(raw)),
	}
	if err := writeBytes(out, raw, md); err != nil {
		return err
	}

	// 6. 最后写路牌。本地 index 只指向我们真正下下来的那个 manifest，
	//    不是远端那个 17 条目的 index —— 本地没有其他平台的字节。
	idx := imageIndex{
		SchemaVersion: 2,
		MediaType:     mtIndex,
		Manifests: []descriptor{{
			MediaType: mtManifest,
			Digest:    manifestDigest,
			Size:      int64(len(raw)),
			Platform:  &platform{OS: "linux", Architecture: runtime.GOARCH},
		}},
	}
	if err := writeLayout(out, idx); err != nil {
		return err
	}
	fmt.Printf("完成: %s\n", out)
	return nil
}

// pickPlatform 从多架构 index 里挑出指定平台。
// 注意：attestation 的 platform 是 unknown/unknown，**不是没有 platform**，
// 所以判据是 os/arch 的值，不是 Platform 是否为 nil。
func pickPlatform(idx imageIndex, goos, arch string) (string, error) {
	for _, d := range idx.Manifests {
		if d.Platform != nil && d.Platform.OS == goos && d.Platform.Architecture == arch {
			return d.Digest, nil
		}
	}
	return "", fmt.Errorf("index 里没有 %s/%s", goos, arch)
}

// outputDir 决定产物写哪：work/output/images/<名字>
//
//	busybox:latest -> work/output/images/busybox（去掉 library/ 前缀和 tag）
func outputDir(r *registry) string {
	name := strings.TrimPrefix(r.repo, "library/")
	return filepath.Join("work", "output", "images", name)
}
