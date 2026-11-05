#!/usr/bin/env python3
"""
Universal Hardware SEL (System Event Log) Prometheus Micro-Exporter
Mendukung: Dell iDRAC 7/8/9, Fujitsu iRMC S4/S5/S6, HPE iLO 4/5/6, Lenovo XCC/Supermicro
"""

import http.server
import urllib.request
import urllib.parse
import json
import ssl
import base64
import os

# ================= KONFIGURASI =================
PORT = int(os.environ.get("EXPORTER_PORT", 9349))
IDRAC_USER = os.environ.get("IDRAC_USER", "YOUR_IDRAC_USER")
IDRAC_PASSWORD = os.environ.get("IDRAC_PASSWORD", "YOUR_IDRAC_PASSWORD")
TIMEOUT = int(os.environ.get("EXPORTER_TIMEOUT", 10))
# ===============================================

# Pre-emptive Basic Auth Header
auth_str = f"{IDRAC_USER}:{IDRAC_PASSWORD}"
b64_auth = base64.b64encode(auth_str.encode()).decode()
headers = {
    "Authorization": f"Basic {b64_auth}",
    "Accept": "application/json"
}

# Bypass SSL Self-Signed Certificate
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

# Daftar Endpoint Log Universal Semua Vendor
ENDPOINTS = [
    # 1. Dell iDRAC 8 (R730, R630, T630)
    "/redfish/v1/Managers/iDRAC.Embedded.1/Logs/Sel",
    # 2. Dell iDRAC 9 (R740, R640, R750, R760) & Standard DMTF
    "/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/Sel/Entries",
    "/redfish/v1/Managers/iDRAC.Embedded.1/LogServices/SEL/Entries",
    "/redfish/v1/Systems/System.Embedded.1/LogServices/Sel/Entries",
    "/redfish/v1/Systems/System.Embedded.1/LogServices/SEL/Entries",
    # 3. Fujitsu iRMC (S4, S5, S6)
    "/redfish/v1/Systems/0/LogServices/SEL/Entries",
    "/redfish/v1/Managers/0/LogServices/SEL/Entries",
    "/redfish/v1/Systems/1/LogServices/SEL/Entries",
    # 4. HPE iLO 4, 5, 6 (Integrated Management Log / IML)
    "/redfish/v1/Systems/1/LogServices/IML/Entries",
    "/redfish/v1/Systems/1/LogServices/EventLog/Entries",
    # 5. Lenovo XCC / Supermicro BMC
    "/redfish/v1/Managers/1/LogServices/SEL/Entries",
    "/redfish/v1/Systems/1/LogServices/StandardLog/Entries"
]

class SELExporterHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        url_parts = urllib.parse.urlparse(self.path)
        if url_parts.path != "/metrics":
            self.send_response(404)
            self.end_headers()
            return

        query_params = urllib.parse.parse_qs(url_parts.query)
        target = query_params.get("target", [None])[0]

        if not target:
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b"Parameter 'target' is required. Contoh: /metrics?target=192.168.1.100\n")
            return

        output = self.scrape_sel(target)
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
        self.end_headers()
        self.wfile.write(output.encode("utf-8"))

    def scrape_sel(self, target):
        lines = [
            "# HELP idrac_sel_latest_event Latest Critical or Warning event from iDRAC/iRMC/iLO SEL",
            "# TYPE idrac_sel_latest_event gauge"
        ]

        entries = []
        # Cari jalur yang cocok secara otomatis (Auto-Discovery)
        for path in ENDPOINTS:
            url = f"https://{target}{path}"
            try:
                req = urllib.request.Request(url, headers=headers)
                with urllib.request.urlopen(req, timeout=TIMEOUT, context=ctx) as response:
                    data = json.loads(response.read().decode("utf-8"))
                    members = data.get("Members", [])
                    if members:
                        entries = members
                        break
            except Exception:
                continue

        if not entries:
            return "\n".join(lines) + "\n"

        # Filter hanya log Critical & Warning
        filtered = [
            e for e in entries 
            if str(e.get("Severity", "")).lower() in ["critical", "warning"]
        ]

        if not filtered:
            return "\n".join(lines) + "\n"

        # Sortir waktu Created paling baru
        filtered.sort(key=lambda x: str(x.get("Created", "")), reverse=True)
        latest = filtered[0]

        severity = latest.get("Severity", "Warning")
        raw_msg = latest.get("Message", "Unknown event")
        # Bersihkan karakter newline dan escape quote agar valid format Prometheus
        clean_msg = raw_msg.replace("\\", "\\\\").replace('"', '\\"').replace("\n", " ").strip()

        lines.append(f'idrac_sel_latest_event{{status="{severity}",severity="{severity}",message="{clean_msg}"}} 1')
        return "\n".join(lines) + "\n"

if __name__ == "__main__":
    server = http.server.ThreadingHTTPServer(("0.0.0.0", PORT), SELExporterHandler)
    print(f"Universal Hardware SEL Exporter running on port {PORT}...")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        server.server_close()
