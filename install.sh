#!/usr/bin/env sh
set -eu

REPO="easonlin2013927/litenest"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/var/lib/litenest"
SERVICE_FILE="/etc/systemd/system/litenest.service"

if [ "$(id -u)" -ne 0 ]; then
  echo "請使用 root 執行，例如：curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | sudo sh" >&2
  exit 1
fi

command -v curl >/dev/null 2>&1 || { echo "需要 curl，請先安裝 curl。" >&2; exit 1; }
command -v tar >/dev/null 2>&1 || { echo "需要 tar，請先安裝 tar。" >&2; exit 1; }

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l|armv7) ARCH="armv7" ;;
  *) echo "不支援的處理器架構：$(uname -m)" >&2; exit 1 ;;
esac

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM
URL="https://github.com/$REPO/releases/latest/download/litenest-linux-$ARCH.tar.gz"

echo "正在下載 LiteNest ($ARCH)…"
curl -fL --retry 3 --connect-timeout 15 "$URL" -o "$TMP_DIR/litenest.tar.gz"
tar -xzf "$TMP_DIR/litenest.tar.gz" -C "$TMP_DIR"
install -m 0755 "$TMP_DIR/litenest" "$INSTALL_DIR/litenest"

if ! id litenest >/dev/null 2>&1; then
  useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin litenest
fi
install -d -m 0750 -o litenest -g litenest "$DATA_DIR"

cat > "$SERVICE_FILE" <<'SERVICE'
[Unit]
Description=LiteNest lightweight private cloud
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=litenest
Group=litenest
ExecStart=/usr/local/bin/litenest -listen :8080 -data /var/lib/litenest
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/litenest
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable --now litenest

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -n "$IP" ] || IP="localhost"
echo
echo "LiteNest 安裝完成！"
echo "開啟：http://$IP:8080"
echo "預設帳號：admin"
echo "預設密碼：admin（登入後請立即更改）"
