#!/usr/bin/env bash
# 手工造一个 3 层的 OCI 镜像，专门用来验证 unpack 的 layer 叠加 + whiteout。
# 全程本地，不联网。每一步都对应 mini-oci 里的一个概念。
set -euo pipefail
cd "$(dirname "$0")/.."          # phase-1-mini-oci/

OUT=work/output/images/whiteout-demo
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

rm -rf "$OUT"
mkdir -p "$OUT/blobs/sha256"

echo "== 1. 造三层的文件内容 =="
mkdir -p "$TMP/l1" "$TMP/l2" "$TMP/l3"
echo "a from layer1"            > "$TMP/l1/a.txt"
echo "b from layer1"            > "$TMP/l1/b.txt"
echo "a from layer2 (overrode)" > "$TMP/l2/a.txt"
echo "c from layer2"            > "$TMP/l2/c.txt"
# 第三层"删除" b.txt：OCI 的删除 = 放一个 .wh.<文件名> 的空标记
: > "$TMP/l3/.wh.b.txt"

# 打一层：产出两组 digest
#   diff_id = 未压缩 tar 的 sha256   -> 写进 config.rootfs.diff_ids
#   blob    = 压缩后   tar 的 sha256 -> 写进 manifest.layers[].digest
rm -f "$TMP/diffids" "$TMP/layers"
mk_layer () {  # $1=目录 $2=层号
  local dir=$1 n=$2 diff_id blob size
  # COPYFILE_DISABLE=1：别打 AppleDouble 伴生文件（._*）—— macOS 的 tar 默认会加上它们，
  # 因为 APFS 上每个文件都带扩展属性。Linux 构建的镜像不会有这些，Docker Desktop 也靠这个开关。
  ( cd "$dir" && COPYFILE_DISABLE=1 tar --no-mac-metadata -cf "$TMP/l$n.tar" . )
  diff_id=$(shasum -a 256 "$TMP/l$n.tar" | awk '{print $1}')
  gzip -n -c "$TMP/l$n.tar" > "$TMP/l$n.tar.gz"     # -n：不带时间戳，结果可复现
  blob=$(shasum -a 256 "$TMP/l$n.tar.gz" | awk '{print $1}')
  size=$(wc -c < "$TMP/l$n.tar.gz" | tr -d ' ')
  cp "$TMP/l$n.tar.gz" "$OUT/blobs/sha256/$blob"
  echo "   layer$n: diff_id=sha256:${diff_id:0:12}…  blob=sha256:${blob:0:12}…  (${size} 字节)"
  echo "sha256:$diff_id" >> "$TMP/diffids"
  printf '    { "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip", "digest": "sha256:%s", "size": %s }' \
    "$blob" "$size" >> "$TMP/layers"
  [ "$n" = 3 ] || printf ',\n' >> "$TMP/layers"
}

echo
echo "== 2. 打三层 =="
mk_layer "$TMP/l1" 1
mk_layer "$TMP/l2" 2
mk_layer "$TMP/l3" 3

echo
echo "== 3. config blob（注意 diff_ids 是**未压缩** tar 的地址）=="
cat > "$TMP/config.json" <<EOF
{
  "architecture": "arm64",
  "os": "linux",
  "config": { "Cmd": ["/bin/sh"] },
  "rootfs": {
    "type": "layers",
    "diff_ids": [ $(sed 's/^/"/;s/$/"/' "$TMP/diffids" | paste -sd, - | sed 's/,/, /g') ]
  }
}
EOF
CONFIG_DIGEST=$(shasum -a 256 "$TMP/config.json" | awk '{print $1}')
CONFIG_SIZE=$(wc -c < "$TMP/config.json" | tr -d ' ')
cp "$TMP/config.json" "$OUT/blobs/sha256/$CONFIG_DIGEST"
echo "   config: sha256:${CONFIG_DIGEST:0:12}… (${CONFIG_SIZE} 字节)"

echo
echo "== 4. manifest blob（引用 config + 三个 layer）=="
cat > "$TMP/manifest.json" <<EOF
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:$CONFIG_DIGEST",
    "size": $CONFIG_SIZE
  },
  "layers": [
$(cat "$TMP/layers")
  ]
}
EOF
MANIFEST_DIGEST=$(shasum -a 256 "$TMP/manifest.json" | awk '{print $1}')
MANIFEST_SIZE=$(wc -c < "$TMP/manifest.json" | tr -d ' ')
cp "$TMP/manifest.json" "$OUT/blobs/sha256/$MANIFEST_DIGEST"
echo "   manifest: sha256:${MANIFEST_DIGEST:0:12}… (${MANIFEST_SIZE} 字节)"

echo
echo "== 5. 路牌 index.json + oci-layout =="
cat > "$OUT/index.json" <<EOF
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:$MANIFEST_DIGEST",
      "size": $MANIFEST_SIZE,
      "platform": { "os": "linux", "architecture": "arm64" }
    }
  ]
}
EOF
printf '{"imageLayoutVersion": "1.0.0"}\n' > "$OUT/oci-layout"

echo
echo "== 6. 成品 =="
find "$OUT" -type f | sort
