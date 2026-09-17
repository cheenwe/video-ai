#!/usr/bin/env bash
# 前端只构建一次；本脚本负责一个目标的 Go 编译与完整安装包。
set -euo pipefail
cd "$(dirname "$0")/.."
target_os=${1:?usage: package.sh linux\|darwin\|windows amd64\|arm64}
target_arch=${2:?missing architecture}
case "$target_os/$target_arch" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64|windows/arm64) ;;
  *) echo 'Unsupported platform' >&2; exit 1 ;;
esac
[[ -f frontend/dist/index.html ]] || { echo 'Run frontend npm ci && npm run build first' >&2; exit 1; }
name="baseplatform-$target_os-$target_arch"
stage="build/packages/$name"
mkdir -p "$stage/frontend" release
cp -R frontend/dist "$stage/frontend/"
cp docs/部署.md "$stage/DEPLOY.md"
binary=baseplatform
if [[ "$target_os" == windows ]]; then
  binary=baseplatform.exe
  cp deploy/start.bat "$stage/start.bat"
else
  cp deploy/start.sh "$stage/start.sh"
  chmod +x "$stage/start.sh"
fi
(cd backend && CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "../$stage/$binary" ./cmd/baseplatform)
if [[ "$target_os" == windows ]]; then
  archive="$name.zip"
  rm -f "release/$archive"
  (cd build/packages && zip -qr "../../release/$archive" "$name")
else
  archive="$name.tar.gz"
  tar -czf "release/$archive" -C build/packages "$name"
fi
(cd release && shasum -a 256 "$archive" > "$archive.sha256")
echo "release/$archive"
