#!/bin/bash
set -Eeuo pipefail

echo "=========================================="
echo "Prism Fusion 启动中..."
echo "=========================================="

# ========== 更新前端运行时配置 ==========
# 只覆盖 UC 相关配置，保留其他配置不变
PLATFORM_CONFIG_FILE="/app/web/platform-config.json"

if [ -f "$PLATFORM_CONFIG_FILE" ]; then
  echo "$(date -Iseconds) [INFO] 更新前端运行时配置..."
  
  # 使用 jq 只修改需要的字段（如果设置了环境变量）
  TEMP_CONFIG=$(mktemp)
  
  jq \
    --arg ucGatewayUrl "${UC_GATEWAY_URL:-}" \
    '
    if $ucGatewayUrl != "" then .UcGatewayUrl = $ucGatewayUrl else . end
    ' "$PLATFORM_CONFIG_FILE" > "$TEMP_CONFIG" && mv "$TEMP_CONFIG" "$PLATFORM_CONFIG_FILE"
  
  # 修复文件权限，确保 app 用户可读
  chmod 644 "$PLATFORM_CONFIG_FILE"
  chown app:app "$PLATFORM_CONFIG_FILE"
  
  echo "$(date -Iseconds) [INFO] 前端配置已更新: $PLATFORM_CONFIG_FILE"
  
  # 显示当前 UC 配置
  UC_URL=$(jq -r '.UcGatewayUrl' "$PLATFORM_CONFIG_FILE")
  echo "  - UC Gateway URL: $UC_URL"
else
  echo "$(date -Iseconds) [WARN] 配置文件不存在: $PLATFORM_CONFIG_FILE"
fi

# ========== 启动服务 ==========
echo "$(date -Iseconds) [INFO] 启动 Supervisor 管理的服务..."
exec /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf
