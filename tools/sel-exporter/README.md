# Universal Hardware SEL (System Event Log) Prometheus Micro-Exporter

Micro-exporter Python ringan tanpa *third-party dependencies* untuk menyedot pesan event log hardware terbaru (**System Event Log / SEL**) dari berbagai merk server enterprise via protokol **Redfish API**.

Exporter ini mengubah event hardware seperti:
* `Drive 3 is removed from disk drive bay 1.`
* `The chassis is open while the power is off.`
* `Fault detected on drive 2 in disk drive bay 1.`

menjadi metrik standar Prometheus:
```text
idrac_sel_latest_event{status="Critical",severity="Critical",message="Drive 3 is removed from disk drive bay 1."} 1
```

---

## 🖥️ Vendor & Hardware yang Didukung (Multi-Vendor Auto-Discovery)

Script ini secara otomatis mendeteksi jalur endpoint log masing-masing vendor:
1. **Dell PowerEdge** (iDRAC 7, iDRAC 8 R730/R630, iDRAC 9 R740/R640/R750/R760)
2. **Fujitsu Primergy** (iRMC S4, iRMC S5, iRMC S6)
3. **HPE ProLiant** (iLO 4, iLO 5, iLO 6 - Integrated Management Log / IML)
4. **Lenovo ThinkSystem** (XClarity Controller / XCC) & **Supermicro BMC**

---

## 📁 Struktur File

```text
D:\idrac-sel-exporter\
├── idrac-sel-exporter.py       # Script utama micro-exporter Python
├── idrac-sel-exporter.service  # Service systemd Linux
├── install.sh                  # Script instalasi otomatis 1-klik
└── README.md                   # Dokumentasi instalasi dan integrasi
```

---

## 🚀 Panduan Instalasi di Server Monitoring (Linux)

### 1. Salin File ke Server Monitoring
Copy file ke server monitoring:
```bash
# Jalankan installer otomatis:
chmod +x install.sh
sudo ./install.sh
```

Atau instalasi manual:
```bash
# 1. Copy script
sudo cp idrac-sel-exporter.py /usr/local/bin/idrac-sel-exporter.py
sudo chmod +x /usr/local/bin/idrac-sel-exporter.py

# 2. Copy systemd unit
sudo cp idrac-sel-exporter.service /etc/systemd/system/idrac-sel-exporter.service

# 3. Aktifkan service
sudo systemctl daemon-reload
sudo systemctl enable --now idrac-sel-exporter
```

### 2. Uji Coba Scrape Manual
Tes tembak ke salah satu IP iDRAC / iRMC:
```bash
curl "http://127.0.0.1:9349/metrics?target=192.168.1.100"
```
Output yang diharapkan:
```text
# HELP idrac_sel_latest_event Latest Critical or Warning event from iDRAC/iRMC/iLO SEL
# TYPE idrac_sel_latest_event gauge
idrac_sel_latest_event{status="Critical",severity="Critical",message="Drive 3 is removed from disk drive bay 1."} 1
```

---

## ⚙️ Integrasi dengan VictoriaMetrics / Prometheus

Tambahkan job scrape ke `/etc/victoria-metrics/scrape-hardware.yml`:

```yaml
  - job_name: 'redfish-sel'
    metrics_path: /metrics
    file_sd_configs:
      - files:
          - '/etc/victoria-metrics/scrape-hardware/*.yml'
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:9349
```

Lalu reload/restart service VictoriaMetrics hardware:
```bash
sudo systemctl restart victoria-metrics-hardware
```

---

## 📊 Query Grafana Dashboard (PromQL)

Gunakan query gabungan ini di panel tabel Grafana (**Query A**). Status dan severity otomatis disinkronkan dengan `idrac_system_health`:

```promql
(
  # 1. STORAGE SUBSYSTEM
  label_replace(idrac_storage_drive_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Storage Drive Degraded/Fault", "", "")
  or
  label_replace(idrac_storage_controller_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Storage Controller Fault", "", "")
  or
  label_replace(idrac_storage_volume_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Virtual Disk / Volume Degraded", "", "")
  or
  label_replace(idrac_storage_enclosure_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Storage Enclosure / Backplane Fault", "", "")
  or
  label_replace(idrac_storage_battery_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "RAID Battery / BBU Degraded", "", "")
  or
  # 2. POWER & SUHU
  label_replace(idrac_power_supply_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Power Supply Fault", "", "")
  or
  label_replace(idrac_power_supply_redundancy_health{ENV=~"$ENV", status!~"(?i)OK|Fully Redundant"}, "issue", "Power Supply Lost Redundancy", "", "")
  or
  label_replace(idrac_sensors_fan_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Cooling Fan Degraded/Fault", "", "")
  or
  label_replace(idrac_sensors_temperature_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Temperature / Overheating Warning", "", "")
  or
  # 3. CPU, RAM & NETWORK
  label_replace(idrac_memory_module_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "RAM Fault / Degraded", "", "")
  or
  label_replace(idrac_cpu_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "CPU Processor Fault", "", "")
  or
  label_replace(idrac_network_interface_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "NIC / Network Port Down", "", "")
  or
  label_replace(idrac_pcie_device_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "PCIe Device Fault", "", "")
  or
  label_replace(idrac_sensor_health{ENV=~"$ENV", status!~"(?i)OK"}, "issue", "Chassis Sensor Fault", "", "")
  or
  # 4. TERBARU DARI SEL LOG (SEVERITY MENGIKUTI idrac_system_health)
  label_replace(
    label_replace(
      idrac_sel_latest_event{ENV=~"$ENV"} 
      * on(instance) group_left(status) (idrac_system_health{ENV=~"$ENV", status!~"(?i)OK"}),
      "issue", "Hardware Event Log Alert", "", ""
    ),
    "id", "$1", "message", "(.*)"
  )
) * on(instance) group_left(bmc_name) (idrac_system_machine_info{ENV=~"$ENV"})
```

---

## 🔒 Konfigurasi Akun & Keamanan
Jika username atau password iDRAC diubah, Anda bisa:
1. Mengubah langsung variabel `IDRAC_USER` dan `IDRAC_PASSWORD` di file `/usr/local/bin/idrac-sel-exporter.py`, atau
2. Menyetel environment variable di `/etc/systemd/system/idrac-sel-exporter.service`:
   ```ini
   Environment=IDRAC_USER=YOUR_IDRAC_USER
   Environment=IDRAC_PASSWORD=YOUR_IDRAC_PASSWORD
   ```
   Lalu jalankan `systemctl daemon-reload && systemctl restart idrac-sel-exporter`.
