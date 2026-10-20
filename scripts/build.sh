#!/usr/bin/env bash
# BeaconTower 一键构建（doc/18）：两套前端 → go build 单二进制。
# go:embed 编译期要求 web/dist 与 web-toy/dist 都存在，本地裸 go build
# 前必须先跑本脚本（或分别 npm run build:toy / build:classic）。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> [1/3] 构建 classic 前端（Sky Glass，默认主题，挂根路径 /）"
npm run build:classic

echo "==> [2/3] 构建 toy 前端（/toy/ 主题）"
npm run build:toy

echo "==> [3/3] 编译后端（嵌入两套 dist）"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o beacontower .
echo "==> 完成: ./beacontower（./data 为运行数据目录）"
