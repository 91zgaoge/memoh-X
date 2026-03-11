#!/bin/bash
#
# 使用代理构建 Docker 镜像的脚本
# 构建完成后自动清理代理环境变量
#
# 用法: ./scripts/build-with-proxy.sh [service_name]
# 示例: ./scripts/build-with-proxy.sh server

set -e

SERVICE="${1:-server}"

echo "========================================"
echo "  使用代理构建: $SERVICE"
echo "========================================"

# 设置代理
export http_proxy="http://ccd:88152353@10.71.252.4:10810"
export https_proxy="http://ccd:88152353@10.71.252.4:10810"
export all_proxy="http://ccd:88152353@10.71.252.4:10810"
export no_proxy="localhost,127.0.0.1,::1,10.0.0.0/8,192.168.0.0/16,172.16.0.0/12"

echo "代理已设置:"
echo "  http_proxy: $http_proxy"
echo ""

# 构建
echo "开始构建..."
docker compose build \
  --build-arg http_proxy="$http_proxy" \
  --build-arg https_proxy="$https_proxy" \
  --build-arg no_proxy="$no_proxy" \
  "$SERVICE"

echo ""
echo "构建完成!"

# 清理代理
echo ""
echo "清理代理环境变量..."
unset http_proxy
unset https_proxy
unset all_proxy
unset HTTP_PROXY
unset HTTPS_PROXY
unset ALL_PROXY
unset no_proxy
unset NO_PROXY

echo "✓ 代理已清理"
echo ""
echo "可以安全地启动服务: docker compose up -d $SERVICE"
