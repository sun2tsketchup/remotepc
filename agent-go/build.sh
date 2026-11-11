#!/usr/bin/env bash
# Build BetagonRemotePC-v<phiên bản>.exe (Windows) từ mã Go. Cần Go 1.22+.
# Phiên bản lấy từ biến Version trong main.go: nhớ tăng nó và ghi CHANGELOG.md mỗi lần phát hành.
set -e
cd "$(dirname "$0")"
VER=$(grep -oP 'var Version = "\K[^"]+' main.go)
rm -rf web && cp -r ../agent/web web   # web app dùng chung với bản Python
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H windowsgui" -o "../BetagonRemotePC-v$VER.exe" .
echo "Đã tạo ../BetagonRemotePC-v$VER.exe"
