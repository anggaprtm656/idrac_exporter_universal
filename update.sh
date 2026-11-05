#!/usr/bin/env bash
set -e

echo "=== 1. Building idrac_exporter binary ==="
go build -o idrac_exporter ./cmd/idrac_exporter

echo "=== 2. Stopping idrac-exporter service ==="
systemctl stop idrac-exporter || true

echo "=== 3. Installing binary to /usr/local/bin/ ==="
cp idrac_exporter /usr/local/bin/idrac_exporter
chmod +x /usr/local/bin/idrac_exporter

echo "=== 4. Starting idrac-exporter service ==="
systemctl start idrac-exporter

echo "=== 5. Service Status ==="
systemctl status idrac-exporter --no-pager
echo ""
echo "✅ idrac_exporter successfully updated and running with SEL support!"
