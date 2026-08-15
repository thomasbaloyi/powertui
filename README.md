# ⚡ PowerTUI

A real-time terminal user interface (TUI) for monitoring total computer power consumption, individual hardware subsystems, per-process power attribution, and gaming metrics — with integrated **Local Weather Forecasting & Cycling Intelligence**.

Available in both **Go (high-performance native binary)** and **Python**.

---

## 🚀 Quick Start

### Go Version (Recommended - 2.2× Faster, 80% Less RAM)
```bash
# Build the standalone binary
go build -o powertui main.go

# Run
./powertui
```

### Python Version
```bash
python3 powertui.py
```

---

## 🏎️ Performance & Benchmarks (Python vs Go)

Benchmarked on Linux (`amd64`, 12th Gen Intel Core i5-12400F):

| Metric | Python | Go | Speedup / Advantage |
|---|---|---|---|
| **`/proc` Process Scraping** (300+ procs) | `6.81 ms` | **`3.04 ms`** | ⚡ **2.24× Faster** |
| **Full Hardware Sampling Loop** | `13.38 ms` | **`9.58 ms`** | ⚡ **1.40× Faster** |
| **Cycling Weather Engine** (3 slots) | `3,647 ns` | **`1,191 ns`** | ⚡ **3.06× Faster** |
| **Runtime Memory (RSS)** | `~28.6 MB` | **`~4.8 MB`** | 📉 **83% Less RAM** |
| **Distribution** | Multiple `.py` scripts | **Single static binary** | Zero dependencies |

To run the benchmarks yourself:
```bash
python3 compare_benchmarks.py
```

---

## ✨ Features

- **🌤️ Local Weather & Cycling Intelligence (Hour-by-Hour & Next Day Morning)**:
  - **Automatic IP Geolocation & Open-Meteo Integration**: Real-time weather and hourly forecasts with zero API keys required.
  - **🚴 Sound Meteorological Cycling Evaluation**:
    - **Current Conditions**: Evaluates temperature, apparent feels-like, wind speed, gusts, precipitation, and road grip.
    - **Next 3 Hours (Hour-by-Hour Prediction)**: Hour-by-hour forecast (`+1h`, `+2h`, `+3h`) with temperature, rain probability, wind/gust speeds, and instant cycling verdicts (`🟢 GOOD`, `🟡 FAIR`, `🔴 POOR`).
    - **Tomorrow Morning Ride Window (5:00 AM – 10:00 AM)**: Evaluates the upcoming morning ride window with an aggregate ride score (0–100) and hourly breakdown (05:00 to 10:00).
    - **Interactive Weather Window (`W` key)**: Full-screen interactive modal with comprehensive meteorological breakdowns and criteria explanation.
- **🎮 Gaming Telemetry & Hardware Performance HUD**:
  - **Active Game Detection**: Automatically detects running Steam games, Proton / Wine titles, Gamescope sessions, and 3D emulators/apps.
  - **GPU Clocks & Voltages**: Live Shader Clock (MHz), Memory Clock (MHz), and Core Voltage (mV).
  - **Thermals**: GPU Edge, GPU Junction / Hotspot, GDDR6 Memory, and CPU Package temperatures.
  - **Bottleneck Analyzer**: Real-time balance analysis showing whether your system is **GPU Bound**, **CPU Bound (Single-Core bottleneck)**, or **Balanced**.
  - **Gaming Running Cost**: Real-time cost in South African Rands per hour of gaming (`R/hr`).
  - **Peak Session Tracking**: Records peak GPU Shader Clocks, CPU Boost Clocks, and Hotspot thermals during your gaming sessions.
- **🔬 Hardware Telemetry & Bandwidth Flow**:
  - **CPU**: Intel & AMD RAPL package, core, and uncore power draw, core loads, frequencies, and temperature.
  - **GPU**: AMD Radeon / NVIDIA GPU power draw, temperatures, fan speeds, clocks, and VRAM memory bus telemetry.
  - **Data & Bandwidth Flow**: Real-time throughput and cumulative session data moved across:
    - 💾 **Storage Drives (NVMe/SSD)**: Live Read/Write MB/s and total GB read & written.
    - 🌐 **Network Interfaces (LAN/WAN)**: Live Rx/Tx MB/s and total GB downloaded & uploaded.
    - 🧠 **RAM Memory Bus**: Active page allocation rates, memory bus throughput in GB/s, and total RAM data processed.
    - 🎮 **GPU VRAM & GDDR6 Bus**: Active memory controller bus % and VRAM data throughput in GB/s.
    - ⚡ **Aggregate System Data Flow**: Total real-time system bus throughput (GB/s) & cumulative data moved.
  - **Battery (Laptops)**: Live charge/discharge rates, battery percentage, and remaining runtime.
- **📈 Real-Time Rolling Graph**: High-resolution Braille (`⡀⣀⣄⣤⣦⣶⣷⣿`) and ASCII block histogram graphs of system power history with auto-scaling.
- **🔥 Top Power-Hungry Processes**: Real-time attribution of watts to individual running processes (sorted by Power, CPU%, Memory%, or PID).
- **📊 Energy, Financial & Carbon Analytics**:
  - Cumulative energy consumed (Watt-Hours and kWh).
  - Peak, minimum, and running average power draw.
  - Real-time running electricity cost measured in **South African Rands (ZAR / `R`)** (default: **R 3.50 / kWh**, customizable via `K` key).
  - Carbon footprint estimate based on South Africa's coal grid intensity (**920 g CO₂ / kWh**).
- **📋 Automatic Shutdown Summary**: Prints a comprehensive session summary on exit with total energy consumed, total cost in Rands, power extremes/averages, subsystem breakdown, top recorded heavy processes, and current weather/cycling forecast.
- **🎨 5 Dynamic Color Themes**: Cyber Neon, Matrix Green, Sunset Amber, Arctic Blue, and High-Contrast Monochrome.

---

## 🚴 Definition of Good Cycling Weather

The cycling evaluation engine scores conditions on a scale of **0 to 100** across 4 key meteorological dimensions:

| Factor | 🟢 GOOD (75 - 100) | 🟡 FAIR (45 - 74) | 🔴 POOR / NO-GO (< 45) |
|---|---|---|---|
| **Precipitation** | 0.0 mm/h, Prob < 25% (Dry roads) | 0.1 - 0.7 mm/h, Prob 25 - 49% (Light drizzle) | ≥ 0.8 mm/h, Prob ≥ 50%, Thunderstorm, Snow, Freezing rain |
| **Wind Speed** | < 20 km/h (Gentle breeze) | 20 - 30 km/h (Moderate resistance) | > 32 km/h (Dangerous crosswinds/drag) |
| **Wind Gusts** | < 35 km/h | 35 - 48 km/h | > 48 km/h (Loss of control hazard) |
| **Apparent Temp** | 12°C – 28°C (Optimal thermal zone) | 9°C – 11°C (Cool/layers) or 29°C – 33°C (Hot) | < 5°C (Freezing / numbness) or > 35°C (Heat exhaustion) |
| **Atmosphere** | Clear / Partly Cloudy / Overcast | Misty / Hazy | Dense Fog, Lightning Storm, Hail |

---

## ⌨️ Keybindings

| Key | Action |
|---|---|
| `Q` / `Ctrl+C` | Quit PowerTUI & print session summary |
| `P` / `Space` | Pause / Resume live sensor sampling |
| `R` | Reset session statistics & refresh weather data |
| `+` / `-` | Increase / decrease refresh rate (0.2s, 0.5s, 1.0s, 2.0s, 5.0s) |
| `C` | Cycle color theme |
| `E` | Cycle PSU efficiency rating |
| `S` | Toggle process sort order (Power, CPU%, Memory%, PID) |
| `G` | Toggle graph mode (High-res Braille vs Block Bar) |
| `W` | **Toggle Weather & Cycling Prediction detail window** |
| `K` | Edit electricity rate (R/kWh) for cost estimation |
| `H` / `?` | Toggle help overlay dialog |

---

## 🛠️ Requirements & Architecture

- **Go 1.20+** or **Python 3.8+**
- Zero external dependencies for Python; standard `golang.org/x/term` for Go.
- Hardware sensor sources:
  - Linux `powercap/intel-rapl` (CPU package / cores)
  - Linux `hwmon` / sysfs (AMDGPU power, temperatures, fans)
  - NVIDIA SMI / NVML (if NVIDIA GPU present)
  - Linux `power_supply` (Battery & AC adapter)
  - `/proc/stat`, `/proc/diskstats`, `/proc/meminfo`, `/proc/[pid]`
- Weather data sources:
  - IP-based Geolocation (`ip-api.com`)
  - Open-Meteo Weather API (`api.open-meteo.com`)

---

## 📄 License

MIT License
