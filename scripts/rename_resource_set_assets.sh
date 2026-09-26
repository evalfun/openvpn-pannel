#!/usr/bin/env bash
# 把 go-bindata 生成的符号统一加上 ResourceSet 前缀，避免与本包内其它 bindata 资源冲突。
# 用法：bash scripts/rename_resource_set_assets.sh internal/ovpn_server/resource_sets_assets.go
# 注意：绑定顺序很重要，必须先替换更长/更具体的名字，再替换裸的 Asset。
set -euo pipefail

file="${1:?用法: rename_resource_set_assets.sh <生成的 go 文件>}"

if [ ! -f "$file" ]; then
    echo "文件不存在: $file" >&2
    exit 1
fi

perl -0pi -e '
s/\bbindataRead\b/resourceSetBindataRead/g;
s/\b_bindata\b/_resourceSetBindata/g;
s/\bbintreeFn\b/resourceSetBintreeFn/g;
s/\bbintree\b/resourceSetBintree/g;
s/\bAssetInfo\b/ResourceSetAssetInfo/g;
s/\bMustAsset\b/ResourceSetMustAsset/g;
s/\bAssetNames\b/ResourceSetAssetNames/g;
s/\bAssetDir\b/ResourceSetAssetDir/g;
s/\bAsset\b/ResourceSetAsset/g;
' "$file"

gofmt -w "$file"
echo "已重命名资源符号: $file"
