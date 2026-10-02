package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func digestOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// blobPath 把 digest 变成 layout 里的路径：
//
// "sha256:abc..." -> <dir>/blobs/sha256/abc..
//
// 注意文件名**不带** "sha256:" 前缀。
func blobPath(dir, digest string) (string, error) {
	algo, enc, ok := strings.Cut(digest, ":")
	if !ok || algo != "sha256" || len(enc) != 64 {
		return "", fmt.Errorf("不支持的 digest: %q", digest)
	}
	return filepath.Join(dir, "blobs", algo, enc), nil
}

// writeBlob 是落盘的核心纪律：**先写临时文件、校验通过、再改名**。
//
// 校验两件事，缺一不可：
//   - 字节数 == decriptor 声明的 size
//   - 内容的 sha256 == descriptor 声明的 digest
//
// 中途失败就什么也不留 --- 保证"目录里存在某个名字"等价于"那个名字的内容一定正确"。
func writeBlob(dir string, rd io.Reader, d descriptor) error {
	final, err := blobPath(dir, d.Digest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(final), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //改名成功之后才会失败，无害；失败时它负责清场

	// 边读边写边算：数据只过一遍（对应 shell 里的 curl -o 加 shasum）
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), rd)
	if err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if n != d.Size {
		return fmt.Errorf("%s 大小不符：声明 %d 字节，实际 %d 字节", d.Digest, d.Size, n)
	}

	if got := "sha256:" + hex.EncodeToString(h.Sum(nil)); got != d.Digest {
		return fmt.Errorf("内容不符：声明 %s，实际 %s", d.Digest, got)
	}
	return os.Rename(tmp.Name(), final)
}

// writeRemoteBlob 从 registry 拉一个 blob 并落盘
//
// 如果本地已经有这个名字和 blob 就直接跳过了 --- 而且**不需要任何校验**：
// 名字就是内容的 sha256，而writeBlob 保证了 "文件以正式名字出现"就等价于"内容正确"。
// 内容寻址最实用的红利就在这里：缓存判断只值一次 os.Stat。
func writeRemoteBlob(r *registry, dir string, d descriptor) error {
	p, err := blobPath(dir, d.Digest)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		fmt.Printf("	%s 已在本地，跳过下载\n", d.Digest)
		return nil
	}

	body, err := r.fetchBlob(d.Digest)
	if err != nil {
		return err
	}
	defer body.Close()
	return writeBlob(dir, body, d)
}

// writeBytes 把内存里的字节落盘（manifest 就是这么存的）。
func writeBytes(dir string, raw []byte, d descriptor) error {
	return writeBlob(dir, bytes.NewReader(raw), d)
}

// writeLayout 写顶层两个"路牌"。
func writeLayout(dir string, idx imageIndex) error {
	raw, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}

	// 固定内容，一字不差（拼错就是 invalid version of OCI layout file）
	return os.WriteFile(filepath.Join(dir, "oci-layout"),
		[]byte("{\"imageLayoutVersion\": \"1.0.0\"}\n"), 0o644)
}
