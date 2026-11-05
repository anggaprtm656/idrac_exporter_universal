#!/bin/bash
# ========================================================
# Script Installer Universal Hardware SEL Exporter
# Jalankan di server monitoring
# ========================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== 1. Copy script exporter ke /usr/local/bin ==="
cp "${SCRIPT_DIR}/idrac-sel-exporter.py" /usr/local/bin/idrac-sel-exporter.py
chmod +x /usr/local/bin/idrac-sel-exporter.py

echo "=== 2. Copy systemd service ==="
cp "${SCRIPT_DIR}/idrac-sel-exporter.service" /etc/systemd/system/idrac-sel-exporter.service

echo "=== 3. Reload systemd & Start Service ==="
systemctl daemon-reload
systemctl enable --now idrac-sel-exporter
systemctl restart idrac-sel-exporter

echo "=== 4. Cek Status Service ==="
systemctl status idrac-sel-exporter --no-pager

echo ""
echo "=========================================================="
echo "Instalasi Selesai! Exporter berjalan di port 9349."
echo "Tes scrape manual:"
echo "curl \"http://127.0.0.1:9349/metrics?target=192.168.1.100\""
echo "=========================================================="
