#!/usr/bin/env bash
# 手搓版 pull：只用 curl + jq + shasum，把一个镜像写成 OCI layout。
# 每一步都写着它对应 mini-oci 里的哪个函数 —— 这就是你接下来要敲的代码。
set -euo pipefail

cd "$(dirname "$0")/.."      # 回到 phase-1-mini-oci/

REPO=library/busybox
TAG=latest
REG=https://registry-1.docker.io
AUTH=https://auth.docker.io/token
ACCEPT="application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json"
OUT=work/output/shell-busybox
TEMP=$(mktemp -d)
trap 'rm -rf "$TEMP"' EXIT

echo "== 0. 准备目录"
rm -rf "$OUT"
mkdir -p "$OUT/blobs/sha256"

echo
echo "== 1. 换 token                    [Go: authenticate]"
TOKEN=$(curl -s "$AUTH?service=registry.docker.io&scope=repository:$REPO:pull" | jq -r .token)
echo "   token 长度 = ${#TOKEN}"

echo
echo "== 2. 拿 tag 指向的东西            [Go: fetchManifest + mediaTypeOf]"
curl -s -H "Authorization: Bearer $TOKEN" -H "Accept: $ACCEPT" \
     -o "$TEMP/top.raw" "$REG/v2/$REPO/manifests/$TAG"
MEDIATYPE=$(jq -r .mediaType "$TEMP/top.raw")
echo "   mediaType = $MEDIATYPE      <- 路牌：是 index 还是 manifest"

echo
echo "== 3. 挑平台                       [Go: pickPlatform]"
if [ "$MEDIATYPE" = "application/vnd.oci.image.index.v1+json" ]; then
  DIGEST=$(jq -r '.manifests[] | select(.platform.os=="linux" and .platform.architecture=="arm64") | .digest' "$TEMP/top.raw")
  echo "   index 里选中 arm64 -> $DIGEST"
  echo
  echo "== 4. 用 digest 拿真正的 manifest  [Go: fetchManifest 第二次]"
  curl -s -H "Authorization: Bearer $TOKEN" -H "Accept: $ACCEPT" \
       -o "$TEMP/manifest.raw" "$REG/v2/$REPO/manifests/$DIGEST"
else
  DIGEST=""
  cp "$TEMP/top.raw" "$TEMP/manifest.raw"
fi

echo
echo "== 5. 自己算 manifest 的地址       [Go: digestOf]"
# manifest 是引用链的根，没有上游替它声明地址 —— 地址就是我们对这段字节算出来的 sha256
MANIFEST_HEX=$(shasum -a 256 "$TEMP/manifest.raw" | awk '{print $1}')
echo "   自己算的 = sha256:$MANIFEST_HEX"
[ -z "$DIGEST" ] || {
  echo "   index 声明 = $DIGEST"
  [ "sha256:$MANIFEST_HEX" = "$DIGEST" ] && echo "   两者相等 -> index 没骗我们（免费的完整性校验）"
}

echo
echo "== 6. 读 manifest 里引用了谁        [Go: 解析成 imageManifest]"
CONFIG_DIGEST=$(jq -r .config.digest "$TEMP/manifest.raw")
CONFIG_SIZE=$(jq -r .config.size "$TEMP/manifest.raw")
LAYER_DIGEST=$(jq -r .layers[0].digest "$TEMP/manifest.raw")
LAYER_SIZE=$(jq -r .layers[0].size "$TEMP/manifest.raw")
echo "   config $CONFIG_DIGEST ($CONFIG_SIZE 字节)"
echo "   layer  $LAYER_DIGEST ($LAYER_SIZE 字节)"

# 下载 + 校验 + 改名的通用动作（= Go 里的 writeBlob：先临时、后校验、再改名）
fetch_blob () {   # $1=digest $2=size
  local d=$1 want=$2 want_hex=${1#sha256:} trace
  # -L 必须加：registry 会 307 到一个预签名的 CDN 地址（Go 的 http.Client 默认跟随重定向，所以代码里看不出来）
  trace=$(curl -s -L -w '%{http_code} (重定向 %{num_redirects} 次)' \
    -H "Authorization: Bearer $TOKEN" -o "$TEMP/blob" "$REG/v2/$REPO/blobs/$d")
  echo "   HTTP $trace"
  local got_size got_hex
  got_size=$(wc -c < "$TEMP/blob" | tr -d ' ')
  got_hex=$(shasum -a 256 "$TEMP/blob" | awk '{print $1}')
  [ "$got_size" = "$want" ] || { echo "   !! 大小不符: 声明 $want 实际 $got_size"; exit 1; }
  [ "$got_hex" = "$want_hex" ] || { echo "   !! 内容不符: 声明 $want_hex 实际 $got_hex"; exit 1; }
  mv "$TEMP/blob" "$OUT/blobs/sha256/$want_hex"   # 校验通过才给它正式的名字
  echo "   校验通过 -> blobs/sha256/$want_hex"
}

echo
echo "== 7. 下 config 并落盘              [Go: fetchBlob + writeBlob]"
fetch_blob "$CONFIG_DIGEST" "$CONFIG_SIZE"

echo
echo "== 8. 下 layer 并落盘               [Go: 同上，只是大一点]"
fetch_blob "$LAYER_DIGEST" "$LAYER_SIZE"

echo
echo "== 9. 写 manifest blob              [Go: writeBytes]"
cp "$TEMP/manifest.raw" "$OUT/blobs/sha256/$MANIFEST_HEX"
echo "   blobs/sha256/$MANIFEST_HEX ($(wc -c < "$TEMP/manifest.raw" | tr -d ' ') 字节)"

echo
echo "== 10. 写路牌 index.json            [Go: writeLayout]"
# 注意：本地 index 只指向我们真正存下来的那个 manifest，
# 不是远端那个 17 平台的 index（其他平台的字节我们没下）
jq -n --arg d "sha256:$MANIFEST_HEX" --argjson s "$(wc -c < "$TEMP/manifest.raw" | tr -d ' ')" '{
  schemaVersion: 2,
  mediaType: "application/vnd.oci.image.index.v1+json",
  manifests: [{
    mediaType: "application/vnd.oci.image.manifest.v1+json",
    digest: $d,
    size: $s,
    platform: {os: "linux", architecture: "arm64"}
  }]
}' > "$OUT/index.json"
printf '{"imageLayoutVersion": "1.0.0"}\n' > "$OUT/oci-layout"

echo
echo "== 11. 成品：应该正好 5 个文件 =="
find "$OUT" -type f | sort
