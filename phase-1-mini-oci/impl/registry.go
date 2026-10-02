package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OCI 的 mediaType 就是“路牌”： 拿到一个 JSON，先看它说自己是什么，
// 才知道该按 index 解析还是按 manifest 解析

const (
	mtIndex    = "application/vnd.oci.image.index.v1+json"
	mtManifest = "application/vnd.oci.image.manifest.v1+json"

	// 请求 manifest 时必须显式高速 registry 我们认识哪些格式，
	// 否则他可能退化成老格式（schema1）而不是给你的 OCI 格式
	acceptManifests = mtIndex + ", " + mtManifest

	registryHost = "https://registry-1.docker.io"
)

// descriptor 是 OCI 里“指向一个 blob“的统一写法
// digest（内容寻址的地址）+ size + mediaType，三者缺一不可。
// index 的 manifests[]、manifest 的 config/layers 全是它。
type descriptor struct {
	MediaType string    `json:"mediaType"`
	Digest    string    `json:"digest"`
	Size      int64     `json:"size"`
	Platform  *platform `json:"platform,omitempty"`
}

type platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type imageIndex struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Manifests     []descriptor `json:"manifests"`
}

type imageManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}

type registry struct {
	repo  string // "library/busybox"
	tag   string // "latest"
	token string // 惰性获取，拿一次复用
	http  *http.Client
}

func newRegistry(ref string) *registry {
	name, tag := ref, "latest"
	// 只有最后一个冒号后面不含 "/" 时才是 tag（“host:500/foo" 那个冒号后面有“/”）
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i:], "/") {
		name, tag = ref[:i], ref[i+1:]
	}
	name = strings.TrimPrefix(name, "docker.io/")
	// 不带 "/" 的短名是“官方镜像”，在 Docker Hub 上都在 library/ 命名空间下
	if !strings.Contains(name, "/") {
		name = "library/" + name
	}
	return &registry{repo: name, tag: tag, http: http.DefaultClient}
}

func (r *registry) manifestURL(ref string) string {
	return fmt.Sprintf("%s/v2/%s/manifests/%s", registryHost, r.repo, ref)
}

func (r *registry) get(rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", acceptManifests)
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	resp.Body.Close()
	if err := r.authenticate(challenge); err != nil {
		return nil, err
	}

	//req 的 Body 是 nil，所以克隆一下重发是安全的
	retry := req.Clone(context.Background())
	retry.Header.Set("Authorization", "Bearer "+r.token)
	return r.http.Do(retry)
}

func (r *registry) authenticate(challenge string) error {
	params, err := parseChallenge(challenge)
	if err != nil {
		return err
	}
	realm, err := url.Parse(params["realm"])
	if err != nil {
		return fmt.Errorf("解析 realm 失败: %w", err)
	}
	q := realm.Query()
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	if s := params["scope"]; s != "" {
		q.Set("scope", s)
	}
	realm.RawQuery = q.Encode()

	resp, err := r.http.Get(realm.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("换取 token 失败: %s", resp.Status)
	}

	var tok struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("解析 token 响应失败: %w", err)
	}
	if tok.Token == "" {
		return fmt.Errorf("token 响应里没有 token 字段")
	}
	r.token = tok.Token
	return nil
}

// parseChallenge 把 `Bearer k="v",k="v"` 拆成 map —— 纯字符串切分，不需要库。
func parseChallenge(h string) (map[string]string, error) {
	h = strings.TrimSpace(h)
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return nil, fmt.Errorf("不认识的认证方式: %q", h)
	}
	params := map[string]string{}
	for _, part := range strings.Split(h[len(prefix):], ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		params[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	if params["realm"] == "" {
		return nil, fmt.Errorf("认证头里没有 realm: %q", h)
	}
	return params, nil
}

// fetchManifest 拉一份 manifest（ref 可以是 tag 也可以是 digest）。
// 返回**原始字节**和 mediaType —— 原始字节很重要，下一步要拿它算 sha256 当文件名。
func (r *registry) fetchManifest(ref string) (raw []byte, mediaType string, err error) {
	resp, err := r.get(r.manifestURL(ref))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("拉取 manifest %s 失败: %s", ref, resp.Status)
	}
	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return raw, mediaTypeOf(raw, resp.Header.Get("Content-Type")), nil
}

func (r *registry) blobURL(digest string) string {
	return fmt.Sprintf("%s/v2/%s/blobs/%s", registryHost, r.repo, digest)
}

// fetchBlob 返回 body 让调用方自己流式读走 --- blob 可能有几百 MB，
// 不能一股脑读进内存
//
// 注意：registry 会 307 重定向到 CDN。http.Client 默认跟随重定向，
// 所以我们这里什么都不用做（curl 得加 -L）。

func (r *registry) fetchBlob(digest string) (io.ReadCloser, error) {
	resp, err := r.get(r.blobURL(digest))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close() // 出错也要关，否则连接泄漏
		return nil, fmt.Errorf("拉取 blob %s 失败：%s", digest, resp.Status)
	}
	return resp.Body, nil
}

// mediaTypeOf 以 JSON 自己声明的 mediaType 为准，HTTP 头只作兜底。
func mediaTypeOf(raw []byte, header string) string {
	var probe struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(raw, &probe); err == nil && probe.MediaType != "" {
		return probe.MediaType
	}
	return strings.TrimSpace(strings.Split(header, ";")[0])
}
