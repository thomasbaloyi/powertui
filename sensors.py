"""
Hardware Power Sensors and Estimators for PowerTUI.
Collects data from RAPL (Intel/AMD), AMD GPU hwmon, NVIDIA SMI,
Battery / Power Supply sysfs, Disk I/O, RAM, and Process statistics.
"""

import os
import glob
import time
import subprocess
import pwd
from dataclasses import dataclass, field
from typing import Dict, List, Optional, Tuple

@dataclass
class CPUStats:
    model: str = "Unknown CPU"
    cores: int = 1
    threads: int = 1
    total_load: float = 0.0
    core_loads: List[float] = field(default_factory=list)
    avg_freq_mhz: float = 0.0
    core_freqs: List[float] = field(default_factory=list)
    temperature_c: Optional[float] = None
    package_power_w: float = 0.0
    core_power_w: float = 0.0
    dram_power_w: float = 0.0
    uncore_power_w: float = 0.0
    is_rapl_available: bool = False
    is_estimated: bool = False

@dataclass
class GPUStats:
    name: str = "Unknown GPU"
    vendor: str = "Unknown"  # amd, nvidia, intel
    power_w: float = 0.0
    power_cap_w: Optional[float] = None
    temperature_c: Optional[float] = None
    fan_rpm: Optional[int] = None
    fan_percent: Optional[float] = None
    core_clock_mhz: Optional[float] = None
    mem_clock_mhz: Optional[float] = None
    vram_used_mb: Optional[float] = None
    vram_total_mb: Optional[float] = None
    utilization_pct: Optional[float] = None
    is_available: bool = False

@dataclass
class BatteryStats:
    present: bool = False
    status: str = "Unknown"  # Charging, Discharging, Full, Not charging
    power_w: float = 0.0
    voltage_v: float = 0.0
    capacity_pct: int = 0
    energy_wh: float = 0.0
    energy_full_wh: float = 0.0
    time_remaining_min: Optional[int] = None

@dataclass
class OtherStats:
    ram_power_w: float = 0.0
    ram_used_gb: float = 0.0
    ram_total_gb: float = 0.0
    disk_power_w: float = 0.0
    disk_read_mbs: float = 0.0
    disk_write_mbs: float = 0.0
    motherboard_power_w: float = 15.0  # Base desktop chipset & peripherals

@dataclass
class BandwidthStats:
    # Storage
    disk_read_mbs: float = 0.0
    disk_write_mbs: float = 0.0
    session_disk_read_bytes: int = 0
    session_disk_write_bytes: int = 0
    
    # Network
    net_rx_mbs: float = 0.0
    net_tx_mbs: float = 0.0
    session_net_rx_bytes: int = 0
    session_net_tx_bytes: int = 0
    
    # Memory (RAM)
    ram_alloc_mbs: float = 0.0
    ram_bandwidth_gbs: float = 0.0
    session_ram_bytes: int = 0
    
    # GPU VRAM & Bus
    gpu_vram_used_mb: float = 0.0
    gpu_vram_total_mb: float = 0.0
    gpu_mem_busy_pct: float = 0.0
    gpu_bandwidth_gbs: float = 0.0
    session_gpu_vram_bytes: int = 0
    
    # Aggregate
    total_throughput_gbs: float = 0.0
    session_total_data_bytes: int = 0

@dataclass
class GamingStats:
    active_game: str = "Desktop / Idle"
    active_game_pid: Optional[int] = None
    is_gaming: bool = False
    
    # GPU performance & thermals
    gpu_busy_pct: float = 0.0
    mem_busy_pct: float = 0.0
    sclk_mhz: float = 0.0
    mclk_mhz: float = 0.0
    voltage_mv: float = 0.0
    temp_edge_c: Optional[float] = None
    temp_junction_c: Optional[float] = None
    temp_mem_c: Optional[float] = None
    
    # CPU gaming stats
    cpu_peak_boost_mhz: float = 0.0
    cpu_peak_core_load_pct: float = 0.0
    
    # Bottleneck & Gaming economics
    bottleneck_status: str = "Desktop / Idle"
    gaming_cost_per_hour_zar: float = 0.0

@dataclass
class ProcessPower:
    pid: int
    name: str
    user: str
    cpu_percent: float
    mem_percent: float
    estimated_watts: float

@dataclass
class SystemPowerSnapshot:
    timestamp: float
    cpu: CPUStats
    gpus: List[GPUStats]
    battery: BatteryStats
    other: OtherStats
    bandwidth: BandwidthStats
    gaming: GamingStats
    measured_dc_w: float
    total_estimated_dc_w: float
    wall_ac_w: float
    psu_efficiency: float
    psu_profile: str
    top_processes: List[ProcessPower] = field(default_factory=list)


PSU_EFFICIENCY_PROFILES = {
    "80+ Titanium": 0.94,
    "80+ Platinum": 0.92,
    "80+ Gold": 0.90,
    "80+ Silver": 0.86,
    "80+ Bronze": 0.82,
    "Standard": 0.80,
    "Raw DC (100%)": 1.00,
}


class SensorCollector:
    def __init__(self):
        self.last_sample_time = time.time()
        
        # CPU RAPL state
        self.rapl_domains: Dict[str, dict] = {}
        self.init_rapl()
        
        # CPU /proc/stat state
        self.last_cpu_times = self.read_proc_stat()
        
        # Disk stats state
        self.last_disk_stats = self.read_disk_stats()
        self.last_disk_time = time.time()
        
        # Bandwidth & Data flow tracking state
        self.session_disk_read_bytes = 0
        self.session_disk_write_bytes = 0
        self.session_net_rx_bytes = 0
        self.session_net_tx_bytes = 0
        self.session_ram_bytes = 0
        self.session_gpu_vram_bytes = 0
        self.last_net_bytes = self.read_net_bytes()
        self.last_vmstat = self.read_vmstat()
        
        # CPU static info
        self.cpu_model, self.cpu_cores, self.cpu_threads = self.detect_cpu_info()
        
        # GPU detection
        self.gpu_devices = self.detect_gpus()
        
        # Process tracking
        self.last_proc_times: Dict[int, Tuple[float, float]] = {}  # pid -> (cpu_time, timestamp)
        
        # User cache
        self.user_cache: Dict[int, str] = {}

    def detect_cpu_info(self) -> Tuple[str, int, int]:
        model = "Unknown CPU"
        cores = 1
        threads = os.cpu_count() or 1
        try:
            with open("/proc/cpuinfo") as f:
                for line in f:
                    if "model name" in line:
                        model = line.split(":", 1)[1].strip()
                    elif "cpu cores" in line:
                        cores = int(line.split(":", 1)[1].strip())
        except Exception:
            pass
        return model, cores, threads

    def init_rapl(self):
        self.rapl_domains = {}
        rapl_paths = glob.glob("/sys/class/powercap/intel-rapl:*")
        # Add subdomains
        for p in list(rapl_paths):
            rapl_paths.extend(glob.glob(f"{p}/intel-rapl:*"))
            
        for path in sorted(set(rapl_paths)):
            name_file = os.path.join(path, "name")
            energy_file = os.path.join(path, "energy_uj")
            max_range_file = os.path.join(path, "max_energy_range_uj")
            if os.path.exists(name_file) and os.path.exists(energy_file):
                try:
                    with open(name_file) as f:
                        name = f.read().strip()
                    with open(energy_file) as f:
                        cur_energy = int(f.read().strip())
                    max_range = 2**32
                    if os.path.exists(max_range_file):
                        with open(max_range_file) as f:
                            max_range = int(f.read().strip())
                    
                    self.rapl_domains[path] = {
                        "name": name,
                        "last_energy": cur_energy,
                        "last_time": time.time(),
                        "max_range": max_range,
                        "power_w": 0.0
                    }
                except Exception:
                    pass

    def read_proc_stat(self) -> Dict[str, Tuple[int, int]]:
        res = {}
        try:
            with open("/proc/stat") as f:
                for line in f:
                    if line.startswith("cpu"):
                        parts = line.split()
                        name = parts[0]
                        user, nice, system, idle, iowait, irq, softirq, steal = map(int, parts[1:9])
                        idle_time = idle + iowait
                        total_time = user + nice + system + idle + iowait + irq + softirq + steal
                        res[name] = (idle_time, total_time)
        except Exception:
            pass
        return res

    def read_disk_stats(self) -> Dict[str, Tuple[int, int]]:
        res = {}
        try:
            with open("/proc/diskstats") as f:
                for line in f:
                    parts = line.split()
                    if len(parts) >= 14:
                        dev = parts[2]
                        # Filter out partitions and loop devices
                        if dev.startswith("loop") or dev.startswith("ram"):
                            continue
                        if dev.startswith("sd") and dev[-1].isdigit():
                            continue
                        if dev.startswith("nvme") and "p" in dev:
                            continue
                        reads_completed = int(parts[3])
                        sectors_read = int(parts[5])
                        writes_completed = int(parts[7])
                        sectors_written = int(parts[9])
                        res[dev] = (sectors_read, sectors_written)
        except Exception:
            pass
        return res

    def read_net_bytes(self) -> Tuple[int, int]:
        rx, tx = 0, 0
        try:
            with open("/proc/net/dev") as f:
                for line in f:
                    if ":" in line:
                        iface, data = line.split(":", 1)
                        iface = iface.strip()
                        if iface in ["lo"] or iface.startswith("virbr") or iface.startswith("docker"):
                            continue
                        parts = data.split()
                        rx += int(parts[0])
                        tx += int(parts[8])
        except Exception:
            pass
        return rx, tx

    def read_vmstat(self) -> Dict[str, int]:
        stats = {}
        try:
            with open("/proc/vmstat") as f:
                for line in f:
                    parts = line.split()
                    if len(parts) == 2:
                        stats[parts[0]] = int(parts[1])
        except Exception:
            pass
        return stats

    def get_bandwidth_stats(self, dt: float) -> BandwidthStats:
        bw = BandwidthStats()
        
        # 1. Drives / Storage
        cur_disk = self.read_disk_stats()
        d_read_bytes = 0
        d_write_bytes = 0
        for dev, (s_read, s_write) in cur_disk.items():
            if dev in self.last_disk_stats:
                p_read, p_write = self.last_disk_stats[dev]
                d_read_bytes += max(0, s_read - p_read) * 512
                d_write_bytes += max(0, s_write - p_write) * 512
        
        bw.disk_read_mbs = (d_read_bytes / (1024.0 * 1024.0)) / dt
        bw.disk_write_mbs = (d_write_bytes / (1024.0 * 1024.0)) / dt
        self.session_disk_read_bytes += d_read_bytes
        self.session_disk_write_bytes += d_write_bytes
        bw.session_disk_read_bytes = self.session_disk_read_bytes
        bw.session_disk_write_bytes = self.session_disk_write_bytes
        
        # 2. Network
        cur_rx, cur_tx = self.read_net_bytes()
        prev_rx, prev_tx = self.last_net_bytes
        d_rx = max(0, cur_rx - prev_rx)
        d_tx = max(0, cur_tx - prev_tx)
        self.last_net_bytes = (cur_rx, cur_tx)
        
        bw.net_rx_mbs = (d_rx / (1024.0 * 1024.0)) / dt
        bw.net_tx_mbs = (d_tx / (1024.0 * 1024.0)) / dt
        self.session_net_rx_bytes += d_rx
        self.session_net_tx_bytes += d_tx
        bw.session_net_rx_bytes = self.session_net_rx_bytes
        bw.session_net_tx_bytes = self.session_net_tx_bytes
        
        # 3. RAM Memory Bandwidth
        cur_vm = self.read_vmstat()
        d_pages = 0
        for k in ["pgalloc_normal", "pgalloc_dma32", "pgpgin", "pgpgout"]:
            if k in cur_vm and k in self.last_vmstat:
                d_pages += max(0, cur_vm[k] - self.last_vmstat[k])
        self.last_vmstat = cur_vm
        
        ram_alloc_bytes = d_pages * 4096
        bw.ram_alloc_mbs = (ram_alloc_bytes / (1024.0 * 1024.0)) / dt
        bw.ram_bandwidth_gbs = (bw.ram_alloc_mbs / 1024.0) + 0.35  # Active allocations + baseline refresh
        ram_total_session_bytes = int((bw.ram_bandwidth_gbs * 1024 * 1024 * 1024) * dt)
        self.session_ram_bytes += ram_total_session_bytes
        bw.session_ram_bytes = self.session_ram_bytes
        
        # 4. GPU VRAM & Memory Bus
        for d in glob.glob("/sys/class/drm/card*/device"):
            if os.path.exists(f"{d}/mem_busy_percent"):
                try:
                    bw.gpu_mem_busy_pct = float(open(f"{d}/mem_busy_percent").read().strip())
                    if os.path.exists(f"{d}/mem_info_vram_used"):
                        bw.gpu_vram_used_mb = float(open(f"{d}/mem_info_vram_used").read().strip()) / (1024*1024)
                    if os.path.exists(f"{d}/mem_info_vram_total"):
                        bw.gpu_vram_total_mb = float(open(f"{d}/mem_info_vram_total").read().strip()) / (1024*1024)
                    # Navi 48 GDDR6 256-bit @ 20Gbps = 640 GB/s peak
                    bw.gpu_bandwidth_gbs = (bw.gpu_mem_busy_pct / 100.0) * 640.0
                    gpu_bytes = int((bw.gpu_bandwidth_gbs * 1024 * 1024 * 1024) * dt)
                    self.session_gpu_vram_bytes += gpu_bytes
                    bw.session_gpu_vram_bytes = self.session_gpu_vram_bytes
                    break
                except Exception:
                    pass
                    
        # 5. Aggregate Totals
        bw.total_throughput_gbs = (bw.disk_read_mbs + bw.disk_write_mbs + bw.net_rx_mbs + bw.net_tx_mbs)/1024.0 + bw.ram_bandwidth_gbs + bw.gpu_bandwidth_gbs
        bw.session_total_data_bytes = (self.session_disk_read_bytes + self.session_disk_write_bytes + 
                                       self.session_net_rx_bytes + self.session_net_tx_bytes + 
                                       self.session_ram_bytes + self.session_gpu_vram_bytes)
        return bw

    def detect_active_game(self) -> Tuple[str, Optional[int]]:
        # Common gaming executables, Steam games, Wine/Proton, Lutris, Heroic
        for p in glob.glob("/proc/[0-9]*"):
            try:
                pid = int(os.path.basename(p))
                cmd_file = os.path.join(p, "cmdline")
                if not os.path.exists(cmd_file):
                    continue
                with open(cmd_file, "rb") as f:
                    cmdline = f.read().decode("utf-8", errors="ignore").replace("\x00", " ").strip()
                if not cmdline:
                    continue
                    
                comm_file = os.path.join(p, "comm")
                comm = open(comm_file).read().strip() if os.path.exists(comm_file) else ""
                
                # Check for Steam common games, gamescope, wine/proton games
                if "steamapps/common/" in cmdline:
                    idx = cmdline.find("steamapps/common/")
                    sub = cmdline[idx + len("steamapps/common/"):].split("/")[0]
                    if sub and not sub.startswith("SteamLinuxRuntime"):
                        return f"Steam: {sub}", pid
                elif "gamescope" in comm or "gamescope" in cmdline:
                    return "Gamescope Session", pid
                elif "wine" in comm.lower() or "proton" in cmdline.lower():
                    for part in cmdline.split():
                        if part.lower().endswith(".exe") and "windows" not in part.lower() and "steam" not in part.lower():
                            return f"Proton: {os.path.basename(part)}", pid
                    if "steamwebhelper" not in comm:
                        return "Proton/Wine Game", pid
                elif comm.lower() in ["cs2", "dota2", "hl2_linux", "valheim.x86_64", "overwatch", "cyberpunk2077", "rpcs3", "yuzu", "ryujinx"]:
                    return comm, pid
            except Exception:
                pass
        return "Desktop / Idle", None

    def get_gaming_stats(self, cpu: CPUStats, gpus: List[GPUStats], wall_w: float, cost_rate: float = 3.50) -> GamingStats:
        gst = GamingStats()
        game_name, game_pid = self.detect_active_game()
        gst.active_game = game_name
        gst.active_game_pid = game_pid
        gst.is_gaming = (game_name != "Desktop / Idle") or (any(g.power_w > 90.0 for g in gpus))
        
        # GPU Telemetry & Thermals (AMD sysfs hwmon)
        for h in glob.glob("/sys/class/hwmon/hwmon*"):
            try:
                name = open(f"{h}/name").read().strip()
                if "amdgpu" in name:
                    # Thermals
                    for i in range(1, 4):
                        label_f = f"{h}/temp{i}_label"
                        input_f = f"{h}/temp{i}_input"
                        if os.path.exists(input_f):
                            t_val = float(open(input_f).read().strip()) / 1000.0
                            lbl = open(label_f).read().strip() if os.path.exists(label_f) else f"temp{i}"
                            if "edge" in lbl:
                                gst.temp_edge_c = t_val
                            elif "junction" in lbl or "hotspot" in lbl:
                                gst.temp_junction_c = t_val
                            elif "mem" in lbl:
                                gst.temp_mem_c = t_val
                    # Clocks
                    if os.path.exists(f"{h}/freq1_input"):
                        gst.sclk_mhz = float(open(f"{h}/freq1_input").read().strip()) / 1_000_000.0
                    if os.path.exists(f"{h}/freq2_input"):
                        gst.mclk_mhz = float(open(f"{h}/freq2_input").read().strip()) / 1_000_000.0
                    # Voltage
                    if os.path.exists(f"{h}/in0_input"):
                        gst.voltage_mv = float(open(f"{h}/in0_input").read().strip())
                    break
            except Exception:
                pass

        # DRM busy percentages
        for d in glob.glob("/sys/class/drm/card*/device"):
            if os.path.exists(f"{d}/gpu_busy_percent"):
                try:
                    gst.gpu_busy_pct = float(open(f"{d}/gpu_busy_percent").read().strip())
                    if os.path.exists(f"{d}/mem_busy_percent"):
                        gst.mem_busy_pct = float(open(f"{d}/mem_busy_percent").read().strip())
                    break
                except Exception:
                    pass
                    
        # CPU gaming metrics
        gst.cpu_peak_boost_mhz = max(cpu.core_freqs) if cpu.core_freqs else cpu.avg_freq_mhz
        gst.cpu_peak_core_load_pct = max(cpu.core_loads) if cpu.core_loads else cpu.total_load
        
        # Bottleneck Analyzer
        if gst.gpu_busy_pct >= 80.0 and gst.gpu_busy_pct >= gst.cpu_peak_core_load_pct + 10.0:
            gst.bottleneck_status = f"GPU Bound ({int(gst.gpu_busy_pct)}% 3D Load)"
        elif gst.cpu_peak_core_load_pct >= 80.0 and gst.cpu_peak_core_load_pct >= gst.gpu_busy_pct + 10.0:
            gst.bottleneck_status = f"CPU Bound ({int(gst.cpu_peak_core_load_pct)}% Single-Core)"
        elif gst.gpu_busy_pct >= 35.0 or gst.cpu_peak_core_load_pct >= 35.0:
            gst.bottleneck_status = f"Balanced (GPU {int(gst.gpu_busy_pct)}% / CPU {int(gst.cpu_peak_core_load_pct)}%)"
        else:
            gst.bottleneck_status = "Idle / Desktop Load"
            
        # Cost per hour of gaming (in ZAR)
        gst.gaming_cost_per_hour_zar = (wall_w / 1000.0) * cost_rate
        return gst

    def detect_gpus(self) -> List[dict]:
        gpus = []
        # Check AMD GPU sysfs
        for h in sorted(glob.glob("/sys/class/hwmon/hwmon*")):
            name_file = os.path.join(h, "name")
            if os.path.exists(name_file):
                try:
                    with open(name_file) as f:
                        name = f.read().strip()
                    if "amdgpu" in name:
                        gpus.append({
                            "type": "amdgpu",
                            "hwmon_path": h,
                            "name": "AMD Radeon GPU"
                        })
                except Exception:
                    pass
                    
        # Check NVIDIA GPU
        try:
            res = subprocess.run(["which", "nvidia-smi"], capture_output=True, text=True)
            if res.returncode == 0:
                gpus.append({
                    "type": "nvidia",
                    "name": "NVIDIA GPU"
                })
        except Exception:
            pass
            
        return gpus

    def get_cpu_stats(self, dt: float) -> CPUStats:
        stats = CPUStats(
            model=self.cpu_model,
            cores=self.cpu_cores,
            threads=self.cpu_threads
        )
        
        # 1. CPU Load
        cur_proc_stat = self.read_proc_stat()
        if "cpu" in cur_proc_stat and "cpu" in self.last_cpu_times:
            idle_0, total_0 = self.last_cpu_times["cpu"]
            idle_1, total_1 = cur_proc_stat["cpu"]
            d_total = max(1, total_1 - total_0)
            d_idle = idle_1 - idle_0
            stats.total_load = max(0.0, min(100.0, (1.0 - (d_idle / d_total)) * 100.0))
            
        # Core loads
        stats.core_loads = []
        for i in range(self.cpu_threads):
            cname = f"cpu{i}"
            if cname in cur_proc_stat and cname in self.last_cpu_times:
                i0, t0 = self.last_cpu_times[cname]
                i1, t1 = cur_proc_stat[cname]
                dt_c = max(1, t1 - t0)
                stats.core_loads.append(max(0.0, min(100.0, (1.0 - ((i1 - i0) / dt_c)) * 100.0)))
        self.last_cpu_times = cur_proc_stat
        
        # 2. CPU Frequencies
        freqs = []
        for cpu_f in sorted(glob.glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")):
            try:
                with open(cpu_f) as f:
                    freqs.append(float(f.read().strip()) / 1000.0)  # MHz
            except Exception:
                pass
        if not freqs:
            try:
                with open("/proc/cpuinfo") as f:
                    for line in f:
                        if "cpu MHz" in line:
                            freqs.append(float(line.split(":", 1)[1].strip()))
            except Exception:
                pass
        stats.core_freqs = freqs
        if freqs:
            stats.avg_freq_mhz = sum(freqs) / len(freqs)
            
        # 3. CPU Temperature
        temps = []
        for h in glob.glob("/sys/class/hwmon/hwmon*"):
            try:
                with open(f"{h}/name") as f:
                    hname = f.read().strip()
                if hname in ["coretemp", "k10temp", "zenpower", "cpu_thermal", "acpitz"]:
                    for tf in glob.glob(f"{h}/temp*_input"):
                        try:
                            with open(tf) as f:
                                tval = float(f.read().strip()) / 1000.0
                                if 10.0 < tval < 115.0:
                                    temps.append(tval)
                        except Exception:
                            pass
            except Exception:
                pass
        if temps:
            stats.temperature_c = max(temps)

        # 4. CPU RAPL Power
        rapl_ok = False
        now = time.time()
        for path, data in self.rapl_domains.items():
            energy_file = os.path.join(path, "energy_uj")
            try:
                with open(energy_file) as f:
                    cur_energy = int(f.read().strip())
                dt_rapl = now - data["last_time"]
                if dt_rapl > 0.05:
                    d_energy = cur_energy - data["last_energy"]
                    if d_energy < 0:
                        d_energy += data["max_range"]
                    power_w = (d_energy / 1_000_000.0) / dt_rapl
                    data["power_w"] = power_w
                    data["last_energy"] = cur_energy
                    data["last_time"] = now
                    rapl_ok = True
                    
                    name = data["name"].lower()
                    if "package" in name or path.endswith("intel-rapl:0"):
                        stats.package_power_w = power_w
                    elif "core" in name:
                        stats.core_power_w = power_w
                    elif "dram" in name:
                        stats.dram_power_w = power_w
                    elif "uncore" in name:
                        stats.uncore_power_w = power_w
            except Exception:
                pass
                
        stats.is_rapl_available = rapl_ok
        if not rapl_ok or stats.package_power_w <= 0.0:
            # Fallback estimation based on CPU load & core count
            # Baseline idle (~5-10W) + load * TDP estimate
            stats.is_estimated = True
            base_idle = 7.0
            tdp = max(35.0, stats.threads * 6.5)
            stats.package_power_w = base_idle + (stats.total_load / 100.0) * (tdp - base_idle)
            stats.core_power_w = stats.package_power_w * 0.8
            stats.uncore_power_w = stats.package_power_w * 0.2
        else:
            if stats.core_power_w > 0 and stats.uncore_power_w == 0:
                stats.uncore_power_w = max(0.0, stats.package_power_w - stats.core_power_w)

        return stats

    def get_gpu_stats(self) -> List[GPUStats]:
        results = []
        for g in self.gpu_devices:
            if g["type"] == "amdgpu":
                gst = GPUStats(name=g["name"], vendor="AMD", is_available=True)
                h = g["hwmon_path"]
                
                # Power
                p_file = os.path.join(h, "power1_average")
                if not os.path.exists(p_file):
                    p_file = os.path.join(h, "power1_input")
                if os.path.exists(p_file):
                    try:
                        with open(p_file) as f:
                            gst.power_w = float(f.read().strip()) / 1_000_000.0
                    except Exception:
                        pass
                        
                # Power cap
                p_cap = os.path.join(h, "power1_cap")
                if os.path.exists(p_cap):
                    try:
                        with open(p_cap) as f:
                            gst.power_cap_w = float(f.read().strip()) / 1_000_000.0
                    except Exception:
                        pass
                        
                # Temperature
                for tname in ["temp1_input", "temp2_input"]:
                    tpath = os.path.join(h, tname)
                    if os.path.exists(tpath):
                        try:
                            with open(tpath) as f:
                                gst.temperature_c = float(f.read().strip()) / 1000.0
                                break
                        except Exception:
                            pass
                            
                # Fan
                fan_in = os.path.join(h, "fan1_input")
                fan_max = os.path.join(h, "fan1_max")
                if os.path.exists(fan_in):
                    try:
                        with open(fan_in) as f:
                            gst.fan_rpm = int(f.read().strip())
                        if os.path.exists(fan_max):
                            with open(fan_max) as f:
                                fmax = int(f.read().strip())
                                if fmax > 0 and gst.fan_rpm is not None:
                                    gst.fan_percent = min(100.0, (gst.fan_rpm / fmax) * 100.0)
                    except Exception:
                        pass
                        
                # Clocks
                f1 = os.path.join(h, "freq1_input")
                f2 = os.path.join(h, "freq2_input")
                if os.path.exists(f1):
                    try:
                        with open(f1) as f:
                            gst.core_clock_mhz = float(f.read().strip()) / 1_000_000.0
                    except Exception:
                        pass
                if os.path.exists(f2):
                    try:
                        with open(f2) as f:
                            gst.mem_clock_mhz = float(f.read().strip()) / 1_000_000.0
                    except Exception:
                        pass
                        
                results.append(gst)
                
            elif g["type"] == "nvidia":
                gst = GPUStats(name=g["name"], vendor="NVIDIA")
                try:
                    cmd = ["nvidia-smi", "--query-gpu=name,power.draw,power.limit,temperature.gpu,fan.speed,clocks.current.graphics,clocks.current.memory,memory.used,memory.total,utilization.gpu", "--format=csv,noheader,nounits"]
                    out = subprocess.check_output(cmd, text=True, timeout=1).strip()
                    parts = [p.strip() for p in out.split(",")]
                    if len(parts) >= 10:
                        gst.name = parts[0]
                        gst.power_w = float(parts[1]) if parts[1] != "[N/A]" else 0.0
                        gst.power_cap_w = float(parts[2]) if parts[2] != "[N/A]" else None
                        gst.temperature_c = float(parts[3]) if parts[3] != "[N/A]" else None
                        gst.fan_percent = float(parts[4]) if parts[4] != "[N/A]" else None
                        gst.core_clock_mhz = float(parts[5]) if parts[5] != "[N/A]" else None
                        gst.mem_clock_mhz = float(parts[6]) if parts[6] != "[N/A]" else None
                        gst.vram_used_mb = float(parts[7]) if parts[7] != "[N/A]" else None
                        gst.vram_total_mb = float(parts[8]) if parts[8] != "[N/A]" else None
                        gst.utilization_pct = float(parts[9]) if parts[9] != "[N/A]" else None
                        gst.is_available = True
                except Exception:
                    pass
                results.append(gst)
                
        return results

    def get_battery_stats(self) -> BatteryStats:
        bst = BatteryStats()
        bats = glob.glob("/sys/class/power_supply/BAT*")
        if not bats:
            # Check AC
            ac_devs = glob.glob("/sys/class/power_supply/AC*") + glob.glob("/sys/class/power_supply/ADP*")
            if ac_devs:
                bst.present = False
                bst.status = "AC Connected (Desktop/Plugged)"
            return bst
            
        bpath = bats[0]
        bst.present = True
        try:
            status_f = os.path.join(bpath, "status")
            if os.path.exists(status_f):
                with open(status_f) as f:
                    bst.status = f.read().strip()
                    
            cap_f = os.path.join(bpath, "capacity")
            if os.path.exists(cap_f):
                with open(cap_f) as f:
                    bst.capacity_pct = int(f.read().strip())
                    
            # Power in micro-Watts
            power_f = os.path.join(bpath, "power_now")
            if os.path.exists(power_f):
                with open(power_f) as f:
                    bst.power_w = float(f.read().strip()) / 1_000_000.0
            else:
                # current_now * voltage_now
                curr_f = os.path.join(bpath, "current_now")
                volt_f = os.path.join(bpath, "voltage_now")
                if os.path.exists(curr_f) and os.path.exists(volt_f):
                    with open(curr_f) as f1, open(volt_f) as f2:
                        curr = float(f1.read().strip()) / 1_000_000.0  # A
                        volt = float(f2.read().strip()) / 1_000_000.0  # V
                        bst.power_w = curr * volt
                        bst.voltage_v = volt
                        
            energy_now_f = os.path.join(bpath, "energy_now")
            energy_full_f = os.path.join(bpath, "energy_full")
            if os.path.exists(energy_now_f) and os.path.exists(energy_full_f):
                with open(energy_now_f) as f1, open(energy_full_f) as f2:
                    bst.energy_wh = float(f1.read().strip()) / 1_000_000.0
                    bst.energy_full_wh = float(f2.read().strip()) / 1_000_000.0
                    
            if bst.power_w > 0.5:
                if bst.status == "Discharging":
                    bst.time_remaining_min = int((bst.energy_wh / bst.power_w) * 60)
                elif bst.status == "Charging" and bst.energy_full_wh > bst.energy_wh:
                    bst.time_remaining_min = int(((bst.energy_full_wh - bst.energy_wh) / bst.power_w) * 60)
        except Exception:
            pass
            
        return bst

    def get_other_stats(self, dt: float) -> OtherStats:
        ost = OtherStats()
        now = time.time()
        
        # RAM usage & power
        try:
            with open("/proc/meminfo") as f:
                mem_total_kb = 0
                mem_avail_kb = 0
                for line in f:
                    if line.startswith("MemTotal:"):
                        mem_total_kb = int(line.split()[1])
                    elif line.startswith("MemAvailable:"):
                        mem_avail_kb = int(line.split()[1])
                ost.ram_total_gb = mem_total_kb / (1024.0 * 1024.0)
                ost.ram_used_gb = (mem_total_kb - mem_avail_kb) / (1024.0 * 1024.0)
                # Estimate: baseline ~1.5W per 8GB stick + load scaling
                num_sticks = max(1, round(ost.ram_total_gb / 8.0))
                ram_util = ost.ram_used_gb / max(1.0, ost.ram_total_gb)
                ost.ram_power_w = num_sticks * (1.2 + ram_util * 1.5)
        except Exception:
            ost.ram_power_w = 3.0

        # Disk I/O & power
        cur_disk_stats = self.read_disk_stats()
        dt_disk = max(0.1, now - self.last_disk_time)
        tot_read_bytes = 0
        tot_write_bytes = 0
        for dev, (s_read, s_write) in cur_disk_stats.items():
            if dev in self.last_disk_stats:
                prev_read, prev_write = self.last_disk_stats[dev]
                tot_read_bytes += max(0, s_read - prev_read) * 512
                tot_write_bytes += max(0, s_write - prev_write) * 512
        self.last_disk_stats = cur_disk_stats
        self.last_disk_time = now
        
        ost.disk_read_mbs = (tot_read_bytes / (1024.0 * 1024.0)) / dt_disk
        ost.disk_write_mbs = (tot_write_bytes / (1024.0 * 1024.0)) / dt_disk
        
        # Disk baseline: ~1.5W idle per drive + 0.05W per MB/s I/O
        num_drives = max(1, len(cur_disk_stats))
        io_total_mbs = ost.disk_read_mbs + ost.disk_write_mbs
        ost.disk_power_w = (num_drives * 1.5) + min(15.0, io_total_mbs * 0.03)

        # Motherboard & Fans baseline: ~14W desktop chipset + VRMs + fans + USB
        ost.motherboard_power_w = 14.0

        return ost

    def get_top_processes(self, cpu_power_w: float, gpu_power_w: float) -> List[ProcessPower]:
        now = time.time()
        procs = []
        new_proc_times = {}
        total_delta_cpu = 0.0

        # Scrape /proc/[0-9]*
        proc_entries = []
        for p in glob.glob("/proc/[0-9]*"):
            try:
                pid = int(os.path.basename(p))
                stat_file = os.path.join(p, "stat")
                with open(stat_file) as f:
                    content = f.read()
                
                # Parse comm
                r_paren = content.rfind(")")
                l_paren = content.find("(")
                if l_paren == -1 or r_paren == -1:
                    continue
                comm = content[l_paren+1:r_paren]
                rest = content[r_paren+2:].split()
                
                utime = int(rest[11])
                stime = int(rest[12])
                tot_ticks = utime + stime
                
                # RSS memory pages
                rss_pages = int(rest[21])
                rss_mb = (rss_pages * os.sysconf("SC_PAGE_SIZE")) / (1024 * 1024)
                
                # UID
                uid = os.stat(stat_file).st_uid
                if uid not in self.user_cache:
                    try:
                        self.user_cache[uid] = pwd.getpwuid(uid).pw_name
                    except Exception:
                        self.user_cache[uid] = str(uid)
                username = self.user_cache[uid]
                
                new_proc_times[pid] = (tot_ticks, now)
                
                if pid in self.last_proc_times:
                    prev_ticks, prev_time = self.last_proc_times[pid]
                    dt = now - prev_time
                    if dt > 0.1:
                        d_ticks = max(0, tot_ticks - prev_ticks)
                        # Ticks to seconds: ticks / HZ
                        hz = os.sysconf("SC_CLK_TCK") or 100
                        cpu_usage_sec = d_ticks / hz
                        norm_cpu_pct = (cpu_usage_sec / (dt * max(1, self.cpu_threads))) * 100.0
                        if norm_cpu_pct > 0.05 or rss_mb > 50:
                            proc_entries.append((pid, comm, username, cpu_usage_sec, dt, rss_mb))
                            total_delta_cpu += cpu_usage_sec
            except Exception:
                pass

        self.last_proc_times = new_proc_times
        
        # Allocate power proportionally
        total_ram_mb = 16384.0
        try:
            with open("/proc/meminfo") as f:
                for line in f:
                    if line.startswith("MemTotal:"):
                        total_ram_mb = float(line.split()[1]) / 1024.0
                        break
        except Exception:
            pass

        all_procs = []
        for pid, comm, username, raw_cpu_usage_sec, dt_proc, rss_mb in proc_entries:
            # Normalized to 0 - 100% total system CPU scale (across all threads)
            cpu_pct = min(100.0, (raw_cpu_usage_sec / (dt_proc * max(1, self.cpu_threads))) * 100.0)
            mem_pct = min(100.0, (rss_mb / total_ram_mb) * 100.0)
            
            # Attribute CPU power proportionally to CPU load and core count
            if total_delta_cpu > 0.01:
                proc_cpu_w = (raw_cpu_usage_sec / max(0.001, total_delta_cpu)) * cpu_power_w
            else:
                proc_cpu_w = (cpu_pct / 100.0) * cpu_power_w
                
            # Base RAM/system active power
            proc_mem_w = (mem_pct / 100.0) * 4.0
            est_watts = max(0.01, proc_cpu_w + proc_mem_w)
            all_procs.append(ProcessPower(
                pid=pid,
                name=comm,
                user=username,
                cpu_percent=cpu_pct,
                mem_percent=mem_pct,
                estimated_watts=est_watts
            ))

        # Split: heavy processes (>= 5.0W) vs other (< 5.0W)
        heavy_procs = [p for p in all_procs if p.estimated_watts >= 5.0]
        other_procs = [p for p in all_procs if p.estimated_watts < 5.0]
        
        # Sort heavy processes descending by power
        heavy_procs.sort(key=lambda x: x.estimated_watts, reverse=True)
        
        # Aggregate all processes below 5W
        if other_procs:
            other_cpu = sum(p.cpu_percent for p in other_procs)
            other_mem = sum(p.mem_percent for p in other_procs)
            other_power = sum(p.estimated_watts for p in other_procs)
            summary_proc = ProcessPower(
                pid=0,
                name=f"Other (<5W, {len(other_procs)} procs)",
                user="system",
                cpu_percent=min(100.0, other_cpu),
                mem_percent=min(100.0, other_mem),
                estimated_watts=other_power
            )
            return heavy_procs + [summary_proc]
            
        return heavy_procs

    def sample(self, psu_profile: str = "80+ Gold") -> SystemPowerSnapshot:
        now = time.time()
        dt = max(0.1, now - self.last_sample_time)
        self.last_sample_time = now
        
        cpu = self.get_cpu_stats(dt)
        gpus = self.get_gpu_stats()
        battery = self.get_battery_stats()
        other = self.get_other_stats(dt)
        bandwidth = self.get_bandwidth_stats(dt)
        
        # Calculate Measured Hardware Sensor DC Power
        gpu_power_sum = sum(g.power_w for g in gpus)
        measured_dc = cpu.package_power_w + gpu_power_sum
        
        # Calculate Total DC (Sensors + RAM + Storage + Board)
        total_estimated_dc = measured_dc + other.ram_power_w + other.disk_power_w + other.motherboard_power_w
        
        # If laptop is on battery discharging, battery sensor is ground truth for wall/battery draw!
        if battery.present and battery.status == "Discharging" and battery.power_w > 0:
            wall_ac = battery.power_w
            efficiency = 1.00
        else:
            efficiency = PSU_EFFICIENCY_PROFILES.get(psu_profile, 0.90)
            wall_ac = total_estimated_dc / efficiency
            
        top_procs = self.get_top_processes(cpu.package_power_w, gpu_power_sum)
        gaming = self.get_gaming_stats(cpu, gpus, wall_ac)
        
        return SystemPowerSnapshot(
            timestamp=now,
            cpu=cpu,
            gpus=gpus,
            battery=battery,
            other=other,
            bandwidth=bandwidth,
            gaming=gaming,
            measured_dc_w=measured_dc,
            total_estimated_dc_w=total_estimated_dc,
            wall_ac_w=wall_ac,
            psu_efficiency=efficiency,
            psu_profile=psu_profile,
            top_processes=top_procs
        )
