package sensors

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CPUStats struct {
	Model           string    `json:"model"`
	Cores           int       `json:"cores"`
	Threads         int       `json:"threads"`
	TotalLoad       float64   `json:"total_load"`
	CoreLoads       []float64 `json:"core_loads"`
	AvgFreqMhz      float64   `json:"avg_freq_mhz"`
	CoreFreqs       []float64 `json:"core_freqs"`
	TemperatureC    *float64  `json:"temperature_c"`
	PackagePowerW   float64   `json:"package_power_w"`
	CorePowerW      float64   `json:"core_power_w"`
	DramPowerW      float64   `json:"dram_power_w"`
	UncorePowerW    float64   `json:"uncore_power_w"`
	IsRAPLAvailable bool      `json:"is_rapl_available"`
	IsEstimated     bool      `json:"is_estimated"`
}

type GPUStats struct {
	Name           string   `json:"name"`
	Vendor         string   `json:"vendor"`
	PowerW         float64  `json:"power_w"`
	PowerCapW      *float64 `json:"power_cap_w"`
	TemperatureC   *float64 `json:"temperature_c"`
	FanRpm         *int     `json:"fan_rpm"`
	FanPercent     *float64 `json:"fan_percent"`
	CoreClockMhz   *float64 `json:"core_clock_mhz"`
	MemClockMhz    *float64 `json:"mem_clock_mhz"`
	VramUsedMb     *float64 `json:"vram_used_mb"`
	VramTotalMb    *float64 `json:"vram_total_mb"`
	UtilizationPct *float64 `json:"utilization_pct"`
	IsAvailable    bool     `json:"is_available"`
}

type BatteryStats struct {
	Present           bool     `json:"present"`
	Status            string   `json:"status"`
	PowerW            float64  `json:"power_w"`
	VoltageV          float64  `json:"voltage_v"`
	CapacityPct       int      `json:"capacity_pct"`
	EnergyWh          float64  `json:"energy_wh"`
	EnergyFullWh      float64  `json:"energy_full_wh"`
	TimeRemainingMin  *int     `json:"time_remaining_min"`
}

type OtherStats struct {
	RamPowerW         float64 `json:"ram_power_w"`
	RamUsedGb         float64 `json:"ram_used_gb"`
	RamTotalGb        float64 `json:"ram_total_gb"`
	DiskPowerW        float64 `json:"disk_power_w"`
	DiskReadMbs       float64 `json:"disk_read_mbs"`
	DiskWriteMbs      float64 `json:"disk_write_mbs"`
	MotherboardPowerW float64 `json:"motherboard_power_w"`
}

type BandwidthStats struct {
	DiskReadMbs           float64 `json:"disk_read_mbs"`
	DiskWriteMbs          float64 `json:"disk_write_mbs"`
	SessionDiskReadBytes  int64   `json:"session_disk_read_bytes"`
	SessionDiskWriteBytes int64   `json:"session_disk_write_bytes"`
	NetRxMbs              float64 `json:"net_rx_mbs"`
	NetTxMbs              float64 `json:"net_tx_mbs"`
	SessionNetRxBytes     int64   `json:"session_net_rx_bytes"`
	SessionNetTxBytes     int64   `json:"session_net_tx_bytes"`
	RamAllocMbs           float64 `json:"ram_alloc_mbs"`
	RamBandwidthGbs       float64 `json:"ram_bandwidth_gbs"`
	SessionRamBytes       int64   `json:"session_ram_bytes"`
	GpuVramUsedMb         float64 `json:"gpu_vram_used_mb"`
	GpuVramTotalMb        float64 `json:"gpu_vram_total_mb"`
	GpuMemBusyPct         float64 `json:"gpu_mem_busy_pct"`
	GpuBandwidthGbs       float64 `json:"gpu_bandwidth_gbs"`
	SessionGpuVramBytes   int64   `json:"session_gpu_vram_bytes"`
	TotalThroughputGbs    float64 `json:"total_throughput_gbs"`
	SessionTotalDataBytes int64   `json:"session_total_data_bytes"`
}

type GamingStats struct {
	ActiveGame           string   `json:"active_game"`
	ActiveGamePid        *int     `json:"active_game_pid"`
	IsGaming             bool     `json:"is_gaming"`
	GpuBusyPct           float64  `json:"gpu_busy_pct"`
	MemBusyPct           float64  `json:"mem_busy_pct"`
	SclkMhz              float64  `json:"sclk_mhz"`
	MclkMhz              float64  `json:"mclk_mhz"`
	VoltageMv            float64  `json:"voltage_mv"`
	TempEdgeC            *float64 `json:"temp_edge_c"`
	TempJunctionC        *float64 `json:"temp_junction_c"`
	TempMemC             *float64 `json:"temp_mem_c"`
	CpuPeakBoostMhz      float64  `json:"cpu_peak_boost_mhz"`
	CpuPeakCoreLoadPct   float64  `json:"cpu_peak_core_load_pct"`
	BottleneckStatus     string   `json:"bottleneck_status"`
	GamingCostPerHourZar float64  `json:"gaming_cost_per_hour_zar"`
}

type ProcessPower struct {
	Pid            int     `json:"pid"`
	Name           string  `json:"name"`
	User           string  `json:"user"`
	CpuPercent     float64 `json:"cpu_percent"`
	MemPercent     float64 `json:"mem_percent"`
	EstimatedWatts float64 `json:"estimated_watts"`
}

type SystemPowerSnapshot struct {
	Timestamp          float64        `json:"timestamp"`
	CPU                CPUStats       `json:"cpu"`
	GPUs               []GPUStats     `json:"gpus"`
	Battery            BatteryStats   `json:"battery"`
	Other              OtherStats     `json:"other"`
	Bandwidth          BandwidthStats `json:"bandwidth"`
	Gaming             GamingStats    `json:"gaming"`
	MeasuredDcW        float64        `json:"measured_dc_w"`
	TotalEstimatedDcW  float64        `json:"total_estimated_dc_w"`
	WallAcW            float64        `json:"wall_ac_w"`
	PsuEfficiency      float64        `json:"psu_efficiency"`
	PsuProfile         string         `json:"psu_profile"`
	TopProcesses       []ProcessPower `json:"top_processes"`
}

var PSUEfficiencyProfiles = map[string]float64{
	"80+ Titanium":   0.94,
	"80+ Platinum":   0.92,
	"80+ Gold":       0.90,
	"80+ Silver":     0.86,
	"80+ Bronze":     0.82,
	"Standard":       0.80,
	"Raw DC (100%)": 1.00,
}

type RaplDomain struct {
	Path       string
	Name       string
	LastEnergy int64
	LastTime   float64
	MaxRange   int64
	PowerW     float64
}

type GPUDeviceInfo struct {
	Type       string
	HwmonPath  string
	Name       string
}

type SensorCollector struct {
	mu                    sync.Mutex
	lastSampleTime        float64
	raplDomains           []*RaplDomain
	lastCPUTimes          map[string][2]int64 // cpu -> [idle, total]
	lastDiskStats         map[string][2]int64 // dev -> [reads, writes]
	lastDiskTime          float64
	lastNetRx             int64
	lastNetTx             int64
	lastVmstat            map[string]int64
	sessionDiskReadBytes  int64
	sessionDiskWriteBytes int64
	sessionNetRxBytes     int64
	sessionNetTxBytes     int64
	sessionRamBytes       int64
	sessionGpuVramBytes   int64

	cpuModel              string
	cpuCores              int
	cpuThreads            int
	gpuDevices            []GPUDeviceInfo
	lastProcTimes         map[int][2]float64 // pid -> [tot_ticks, timestamp]
	userCache             map[int]string
}

func NewSensorCollector() *SensorCollector {
	sc := &SensorCollector{
		lastSampleTime: float64(time.Now().UnixNano()) / 1e9,
		lastDiskTime:   float64(time.Now().UnixNano()) / 1e9,
		userCache:      make(map[int]string),
		lastProcTimes:  make(map[int][2]float64),
	}

	sc.cpuModel, sc.cpuCores, sc.cpuThreads = sc.detectCPUInfo()
	sc.initRAPL()
	sc.lastCPUTimes = sc.readProcStat()
	sc.lastDiskStats = sc.readDiskStats()
	sc.lastNetRx, sc.lastNetTx = sc.readNetBytes()
	sc.lastVmstat = sc.readVmstat()
	sc.gpuDevices = sc.detectGPUs()

	return sc
}

func (sc *SensorCollector) detectCPUInfo() (string, int, int) {
	model := "Unknown CPU"
	cores := 1
	threads := 1

	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		tCount := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 && model == "Unknown CPU" {
					model = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "cpu cores") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					if c, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
						cores = c
					}
				}
			} else if strings.HasPrefix(line, "processor") {
				tCount++
			}
		}
		if tCount > 0 {
			threads = tCount
		}
	}
	if cores > threads {
		threads = cores
	}
	return model, cores, threads
}

func (sc *SensorCollector) initRAPL() {
	sc.raplDomains = nil
	paths, _ := filepath.Glob("/sys/class/powercap/intel-rapl:*")
	for _, p := range paths {
		sub, _ := filepath.Glob(filepath.Join(p, "intel-rapl:*"))
		paths = append(paths, sub...)
	}

	seen := make(map[string]bool)
	now := float64(time.Now().UnixNano()) / 1e9

	for _, p := range paths {
		if seen[p] {
			continue
		}
		seen[p] = true

		nameBytes, err1 := os.ReadFile(filepath.Join(p, "name"))
		energyBytes, err2 := os.ReadFile(filepath.Join(p, "energy_uj"))
		if err1 == nil && err2 == nil {
			curEnergy, _ := strconv.ParseInt(strings.TrimSpace(string(energyBytes)), 10, 64)
			maxRange := int64(1 << 32)
			if rangeBytes, err := os.ReadFile(filepath.Join(p, "max_energy_range_uj")); err == nil {
				if mr, err := strconv.ParseInt(strings.TrimSpace(string(rangeBytes)), 10, 64); err == nil {
					maxRange = mr
				}
			}
			sc.raplDomains = append(sc.raplDomains, &RaplDomain{
				Path:       p,
				Name:       strings.TrimSpace(string(nameBytes)),
				LastEnergy: curEnergy,
				LastTime:   now,
				MaxRange:   maxRange,
			})
		}
	}
}

func (sc *SensorCollector) readProcStat() map[string][2]int64 {
	res := make(map[string][2]int64)
	f, err := os.Open("/proc/stat")
	if err != nil {
		return res
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu") {
			parts := strings.Fields(line)
			if len(parts) >= 8 {
				name := parts[0]
				var fields [8]int64
				for i := 0; i < 8 && i+1 < len(parts); i++ {
					fields[i], _ = strconv.ParseInt(parts[i+1], 10, 64)
				}
				idle := fields[3] + fields[4] // idle + iowait
				total := fields[0] + fields[1] + fields[2] + fields[3] + fields[4] + fields[5] + fields[6] + fields[7]
				res[name] = [2]int64{idle, total}
			}
		}
	}
	return res
}

func (sc *SensorCollector) readDiskStats() map[string][2]int64 {
	res := make(map[string][2]int64)
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return res
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 14 {
			dev := parts[2]
			if strings.HasPrefix(dev, "loop") || strings.HasPrefix(dev, "ram") {
				continue
			}
			if strings.HasPrefix(dev, "sd") && len(dev) > 3 && dev[len(dev)-1] >= '0' && dev[len(dev)-1] <= '9' {
				continue
			}
			if strings.HasPrefix(dev, "nvme") && strings.Contains(dev, "p") {
				continue
			}
			secRead, _ := strconv.ParseInt(parts[5], 10, 64)
			secWrite, _ := strconv.ParseInt(parts[9], 10, 64)
			res[dev] = [2]int64{secRead, secWrite}
		}
	}
	return res
}

func (sc *SensorCollector) readNetBytes() (int64, int64) {
	var rx, tx int64
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return rx, tx
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.Index(line, ":"); idx != -1 {
			iface := strings.TrimSpace(line[:idx])
			if iface == "lo" || strings.HasPrefix(iface, "virbr") || strings.HasPrefix(iface, "docker") {
				continue
			}
			parts := strings.Fields(line[idx+1:])
			if len(parts) >= 9 {
				r, _ := strconv.ParseInt(parts[0], 10, 64)
				t, _ := strconv.ParseInt(parts[8], 10, 64)
				rx += r
				tx += t
			}
		}
	}
	return rx, tx
}

func (sc *SensorCollector) readVmstat() map[string]int64 {
	res := make(map[string]int64)
	f, err := os.Open("/proc/vmstat")
	if err != nil {
		return res
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 2 {
			val, _ := strconv.ParseInt(parts[1], 10, 64)
			res[parts[0]] = val
		}
	}
	return res
}

func (sc *SensorCollector) detectGPUs() []GPUDeviceInfo {
	var gpus []GPUDeviceInfo
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hwmons {
		if nameB, err := os.ReadFile(filepath.Join(h, "name")); err == nil {
			name := strings.TrimSpace(string(nameB))
			if strings.Contains(name, "amdgpu") {
				gpus = append(gpus, GPUDeviceInfo{
					Type:      "amdgpu",
					HwmonPath: h,
					Name:      "AMD Radeon GPU",
				})
			}
		}
	}

	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		gpus = append(gpus, GPUDeviceInfo{
			Type: "nvidia",
			Name: "NVIDIA GPU",
		})
	}
	return gpus
}

func (sc *SensorCollector) GetCPUStats(dt float64) CPUStats {
	stats := CPUStats{
		Model:   sc.cpuModel,
		Cores:   sc.cpuCores,
		Threads: sc.cpuThreads,
	}

	// 1. CPU Load
	curStat := sc.readProcStat()
	if c1, ok1 := curStat["cpu"]; ok1 {
		if c0, ok0 := sc.lastCPUTimes["cpu"]; ok0 {
			dTotal := math.Max(1.0, float64(c1[1]-c0[1]))
			dIdle := float64(c1[0] - c0[0])
			stats.TotalLoad = math.Max(0.0, math.Min(100.0, (1.0-(dIdle/dTotal))*100.0))
		}
	}

	for i := 0; i < sc.cpuThreads; i++ {
		cname := fmt.Sprintf("cpu%d", i)
		if c1, ok1 := curStat[cname]; ok1 {
			if c0, ok0 := sc.lastCPUTimes[cname]; ok0 {
				dtC := math.Max(1.0, float64(c1[1]-c0[1]))
				dIdle := float64(c1[0] - c0[0])
				stats.CoreLoads = append(stats.CoreLoads, math.Max(0.0, math.Min(100.0, (1.0-(dIdle/dtC))*100.0)))
			}
		}
	}
	sc.lastCPUTimes = curStat

	// 2. CPU Frequencies
	var freqs []float64
	freqFiles, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	for _, f := range freqFiles {
		if b, err := os.ReadFile(f); err == nil {
			if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
				freqs = append(freqs, val/1000.0) // MHz
			}
		}
	}
	if len(freqs) == 0 {
		if f, err := os.Open("/proc/cpuinfo"); err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "cpu MHz") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						if val, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
							freqs = append(freqs, val)
						}
					}
				}
			}
			f.Close()
		}
	}
	stats.CoreFreqs = freqs
	if len(freqs) > 0 {
		sum := 0.0
		for _, f := range freqs {
			sum += f
		}
		stats.AvgFreqMhz = sum / float64(len(freqs))
	}

	// 3. CPU Temperature
	var temps []float64
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hwmons {
		if nameB, err := os.ReadFile(filepath.Join(h, "name")); err == nil {
			hname := strings.TrimSpace(string(nameB))
			if hname == "coretemp" || hname == "k10temp" || hname == "zenpower" || hname == "cpu_thermal" || hname == "acpitz" {
				tInputs, _ := filepath.Glob(filepath.Join(h, "temp*_input"))
				for _, tf := range tInputs {
					if b, err := os.ReadFile(tf); err == nil {
						if tval, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
							tC := tval / 1000.0
							if tC > 10.0 && tC < 115.0 {
								temps = append(temps, tC)
							}
						}
					}
				}
			}
		}
	}
	if len(temps) > 0 {
		maxT := temps[0]
		for _, t := range temps {
			if t > maxT {
				maxT = t
			}
		}
		stats.TemperatureC = &maxT
	}

	// 4. RAPL Power
	raplOk := false
	now := float64(time.Now().UnixNano()) / 1e9
	for _, domain := range sc.raplDomains {
		energyFile := filepath.Join(domain.Path, "energy_uj")
		if b, err := os.ReadFile(energyFile); err == nil {
			if curEnergy, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
				dtRapl := now - domain.LastTime
				if dtRapl > 0.05 {
					dEnergy := curEnergy - domain.LastEnergy
					if dEnergy < 0 {
						dEnergy += domain.MaxRange
					}
					powerW := (float64(dEnergy) / 1000000.0) / dtRapl
					domain.PowerW = powerW
					domain.LastEnergy = curEnergy
					domain.LastTime = now
					raplOk = true

					name := strings.ToLower(domain.Name)
					if strings.Contains(name, "package") || strings.HasSuffix(domain.Path, "intel-rapl:0") {
						stats.PackagePowerW = powerW
					} else if strings.Contains(name, "core") {
						stats.CorePowerW = powerW
					} else if strings.Contains(name, "dram") {
						stats.DramPowerW = powerW
					} else if strings.Contains(name, "uncore") {
						stats.UncorePowerW = powerW
					}
				}
			}
		}
	}

	stats.IsRAPLAvailable = raplOk
	if !raplOk || stats.PackagePowerW <= 0.0 {
		stats.IsEstimated = true
		baseIdle := 7.0
		tdp := math.Max(35.0, float64(stats.Threads)*6.5)
		stats.PackagePowerW = baseIdle + (stats.TotalLoad/100.0)*(tdp-baseIdle)
		stats.CorePowerW = stats.PackagePowerW * 0.8
		stats.UncorePowerW = stats.PackagePowerW * 0.2
	} else {
		if stats.CorePowerW > 0 && stats.UncorePowerW == 0 {
			stats.UncorePowerW = math.Max(0.0, stats.PackagePowerW-stats.CorePowerW)
		}
	}

	return stats
}

func (sc *SensorCollector) GetGPUStats() []GPUStats {
	var results []GPUStats
	for _, g := range sc.gpuDevices {
		if g.Type == "amdgpu" {
			gst := GPUStats{Name: g.Name, Vendor: "AMD", IsAvailable: true}
			h := g.HwmonPath

			// Power
			pFile := filepath.Join(h, "power1_average")
			if _, err := os.Stat(pFile); os.IsNotExist(err) {
				pFile = filepath.Join(h, "power1_input")
			}
			if b, err := os.ReadFile(pFile); err == nil {
				if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
					gst.PowerW = val / 1000000.0
				}
			}

			// Power cap
			if b, err := os.ReadFile(filepath.Join(h, "power1_cap")); err == nil {
				if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
					pCap := val / 1000000.0
					gst.PowerCapW = &pCap
				}
			}

			// Temperature
			for _, tname := range []string{"temp1_input", "temp2_input"} {
				if b, err := os.ReadFile(filepath.Join(h, tname)); err == nil {
					if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
						tC := val / 1000.0
						gst.TemperatureC = &tC
						break
					}
				}
			}

			// Fan
			if b, err := os.ReadFile(filepath.Join(h, "fan1_input")); err == nil {
				if val, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
					gst.FanRpm = &val
					if maxB, err := os.ReadFile(filepath.Join(h, "fan1_max")); err == nil {
						if fmax, err := strconv.Atoi(strings.TrimSpace(string(maxB))); err == nil && fmax > 0 {
							pct := math.Min(100.0, (float64(val)/float64(fmax))*100.0)
							gst.FanPercent = &pct
						}
					}
				}
			}

			// Clocks
			if b, err := os.ReadFile(filepath.Join(h, "freq1_input")); err == nil {
				if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
					cMhz := val / 1000000.0
					gst.CoreClockMhz = &cMhz
				}
			}
			if b, err := os.ReadFile(filepath.Join(h, "freq2_input")); err == nil {
				if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
					mMhz := val / 1000000.0
					gst.MemClockMhz = &mMhz
				}
			}
			results = append(results, gst)
		} else if g.Type == "nvidia" {
			gst := GPUStats{Name: g.Name, Vendor: "NVIDIA"}
			cmd := exec.Command("nvidia-smi", "--query-gpu=name,power.draw,power.limit,temperature.gpu,fan.speed,clocks.current.graphics,clocks.current.memory,memory.used,memory.total,utilization.gpu", "--format=csv,noheader,nounits")
			if out, err := cmd.Output(); err == nil {
				parts := strings.Split(strings.TrimSpace(string(out)), ",")
				if len(parts) >= 10 {
					gst.Name = strings.TrimSpace(parts[0])
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
						gst.PowerW = v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64); err == nil {
						gst.PowerCapW = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64); err == nil {
						gst.TemperatureC = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64); err == nil {
						gst.FanPercent = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64); err == nil {
						gst.CoreClockMhz = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[6]), 64); err == nil {
						gst.MemClockMhz = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[7]), 64); err == nil {
						gst.VramUsedMb = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[8]), 64); err == nil {
						gst.VramTotalMb = &v
					}
					if v, err := strconv.ParseFloat(strings.TrimSpace(parts[9]), 64); err == nil {
						gst.UtilizationPct = &v
					}
					gst.IsAvailable = true
				}
			}
			results = append(results, gst)
		}
	}
	return results
}

func (sc *SensorCollector) GetBatteryStats() BatteryStats {
	bst := BatteryStats{}
	bats, _ := filepath.Glob("/sys/class/power_supply/BAT*")
	if len(bats) == 0 {
		acDevs, _ := filepath.Glob("/sys/class/power_supply/AC*")
		adpDevs, _ := filepath.Glob("/sys/class/power_supply/ADP*")
		if len(acDevs) > 0 || len(adpDevs) > 0 {
			bst.Present = false
			bst.Status = "AC Connected (Desktop/Plugged)"
		}
		return bst
	}

	bpath := bats[0]
	bst.Present = true

	if b, err := os.ReadFile(filepath.Join(bpath, "status")); err == nil {
		bst.Status = strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile(filepath.Join(bpath, "capacity")); err == nil {
		bst.CapacityPct, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	if b, err := os.ReadFile(filepath.Join(bpath, "power_now")); err == nil {
		if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
			bst.PowerW = val / 1000000.0
		}
	} else {
		curB, err1 := os.ReadFile(filepath.Join(bpath, "current_now"))
		voltB, err2 := os.ReadFile(filepath.Join(bpath, "voltage_now"))
		if err1 == nil && err2 == nil {
			cVal, _ := strconv.ParseFloat(strings.TrimSpace(string(curB)), 64)
			vVal, _ := strconv.ParseFloat(strings.TrimSpace(string(voltB)), 64)
			curr := cVal / 1000000.0
			volt := vVal / 1000000.0
			bst.PowerW = curr * volt
			bst.VoltageV = volt
		}
	}

	enNowB, err1 := os.ReadFile(filepath.Join(bpath, "energy_now"))
	enFullB, err2 := os.ReadFile(filepath.Join(bpath, "energy_full"))
	if err1 == nil && err2 == nil {
		enNow, _ := strconv.ParseFloat(strings.TrimSpace(string(enNowB)), 64)
		enFull, _ := strconv.ParseFloat(strings.TrimSpace(string(enFullB)), 64)
		bst.EnergyWh = enNow / 1000000.0
		bst.EnergyFullWh = enFull / 1000000.0
	}

	if bst.PowerW > 0.5 {
		if bst.Status == "Discharging" {
			rem := int((bst.EnergyWh / bst.PowerW) * 60.0)
			bst.TimeRemainingMin = &rem
		} else if bst.Status == "Charging" && bst.EnergyFullWh > bst.EnergyWh {
			rem := int(((bst.EnergyFullWh - bst.EnergyWh) / bst.PowerW) * 60.0)
			bst.TimeRemainingMin = &rem
		}
	}

	return bst
}

func (sc *SensorCollector) GetOtherStats(dt float64) OtherStats {
	ost := OtherStats{}
	now := float64(time.Now().UnixNano()) / 1e9

	// RAM
	if f, err := os.Open("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(f)
		var memTotalKb, memAvailKb float64
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					memTotalKb, _ = strconv.ParseFloat(parts[1], 64)
				}
			} else if strings.HasPrefix(line, "MemAvailable:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					memAvailKb, _ = strconv.ParseFloat(parts[1], 64)
				}
			}
		}
		f.Close()
		ost.RamTotalGb = memTotalKb / (1024.0 * 1024.0)
		ost.RamUsedGb = (memTotalKb - memAvailKb) / (1024.0 * 1024.0)
		numSticks := math.Max(1.0, math.Round(ost.RamTotalGb/8.0))
		ramUtil := ost.RamUsedGb / math.Max(1.0, ost.RamTotalGb)
		ost.RamPowerW = numSticks * (1.2 + ramUtil*1.5)
	} else {
		ost.RamPowerW = 3.0
	}

	// Disk
	curDisk := sc.readDiskStats()
	dtDisk := math.Max(0.1, now-sc.lastDiskTime)
	var totReadBytes, totWriteBytes int64
	for dev, curVal := range curDisk {
		if prevVal, ok := sc.lastDiskStats[dev]; ok {
			if curVal[0] >= prevVal[0] {
				totReadBytes += (curVal[0] - prevVal[0]) * 512
			}
			if curVal[1] >= prevVal[1] {
				totWriteBytes += (curVal[1] - prevVal[1]) * 512
			}
		}
	}
	sc.lastDiskStats = curDisk
	sc.lastDiskTime = now

	ost.DiskReadMbs = (float64(totReadBytes) / (1024.0 * 1024.0)) / dtDisk
	ost.DiskWriteMbs = (float64(totWriteBytes) / (1024.0 * 1024.0)) / dtDisk

	numDrives := math.Max(1.0, float64(len(curDisk)))
	ioTotal := ost.DiskReadMbs + ost.DiskWriteMbs
	ost.DiskPowerW = (numDrives * 1.5) + math.Min(15.0, ioTotal*0.03)
	ost.MotherboardPowerW = 14.0

	return ost
}

func (sc *SensorCollector) GetBandwidthStats(dt float64) BandwidthStats {
	bw := BandwidthStats{}

	// 1. Storage
	curDisk := sc.readDiskStats()
	var dReadBytes, dWriteBytes int64
	for dev, curVal := range curDisk {
		if prevVal, ok := sc.lastDiskStats[dev]; ok {
			if curVal[0] >= prevVal[0] {
				dReadBytes += (curVal[0] - prevVal[0]) * 512
			}
			if curVal[1] >= prevVal[1] {
				dWriteBytes += (curVal[1] - prevVal[1]) * 512
			}
		}
	}
	bw.DiskReadMbs = (float64(dReadBytes) / (1024.0 * 1024.0)) / dt
	bw.DiskWriteMbs = (float64(dWriteBytes) / (1024.0 * 1024.0)) / dt
	sc.sessionDiskReadBytes += dReadBytes
	sc.sessionDiskWriteBytes += dWriteBytes
	bw.SessionDiskReadBytes = sc.sessionDiskReadBytes
	bw.SessionDiskWriteBytes = sc.sessionDiskWriteBytes

	// 2. Network
	curRx, curTx := sc.readNetBytes()
	dRx := int64(0)
	if curRx >= sc.lastNetRx {
		dRx = curRx - sc.lastNetRx
	}
	dTx := int64(0)
	if curTx >= sc.lastNetTx {
		dTx = curTx - sc.lastNetTx
	}
	sc.lastNetRx, sc.lastNetTx = curRx, curTx

	bw.NetRxMbs = (float64(dRx) / (1024.0 * 1024.0)) / dt
	bw.NetTxMbs = (float64(dTx) / (1024.0 * 1024.0)) / dt
	sc.sessionNetRxBytes += dRx
	sc.sessionNetTxBytes += dTx
	bw.SessionNetRxBytes = sc.sessionNetRxBytes
	bw.SessionNetTxBytes = sc.sessionNetTxBytes

	// 3. RAM
	curVm := sc.readVmstat()
	var dPages int64
	for _, k := range []string{"pgalloc_normal", "pgalloc_dma32", "pgpgin", "pgpgout"} {
		if cVal, ok1 := curVm[k]; ok1 {
			if pVal, ok2 := sc.lastVmstat[k]; ok2 && cVal >= pVal {
				dPages += cVal - pVal
			}
		}
	}
	sc.lastVmstat = curVm

	ramAllocBytes := dPages * 4096
	bw.RamAllocMbs = (float64(ramAllocBytes) / (1024.0 * 1024.0)) / dt
	bw.RamBandwidthGbs = (bw.RamAllocMbs / 1024.0) + 0.35
	ramTotalSessionBytes := int64((bw.RamBandwidthGbs * 1024 * 1024 * 1024) * dt)
	sc.sessionRamBytes += ramTotalSessionBytes
	bw.SessionRamBytes = sc.sessionRamBytes

	// 4. GPU VRAM & Bus
	drmDevices, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, d := range drmDevices {
		if b, err := os.ReadFile(filepath.Join(d, "mem_busy_percent")); err == nil {
			if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
				bw.GpuMemBusyPct = val
				if vramUsedB, err := os.ReadFile(filepath.Join(d, "mem_info_vram_used")); err == nil {
					if vu, err := strconv.ParseFloat(strings.TrimSpace(string(vramUsedB)), 64); err == nil {
						bw.GpuVramUsedMb = vu / (1024 * 1024)
					}
				}
				if vramTotB, err := os.ReadFile(filepath.Join(d, "mem_info_vram_total")); err == nil {
					if vt, err := strconv.ParseFloat(strings.TrimSpace(string(vramTotB)), 64); err == nil {
						bw.GpuVramTotalMb = vt / (1024 * 1024)
					}
				}
				bw.GpuBandwidthGbs = (bw.GpuMemBusyPct / 100.0) * 640.0
				gpuBytes := int64((bw.GpuBandwidthGbs * 1024 * 1024 * 1024) * dt)
				sc.sessionGpuVramBytes += gpuBytes
				bw.SessionGpuVramBytes = sc.sessionGpuVramBytes
				break
			}
		}
	}

	// 5. Aggregate
	bw.TotalThroughputGbs = (bw.DiskReadMbs+bw.DiskWriteMbs+bw.NetRxMbs+bw.NetTxMbs)/1024.0 + bw.RamBandwidthGbs + bw.GpuBandwidthGbs
	bw.SessionTotalDataBytes = sc.sessionDiskReadBytes + sc.sessionDiskWriteBytes + sc.sessionNetRxBytes + sc.sessionNetTxBytes + sc.sessionRamBytes + sc.sessionGpuVramBytes
	return bw
}

func (sc *SensorCollector) DetectActiveGame() (string, *int) {
	procDirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, p := range procDirs {
		pidStr := filepath.Base(p)
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		cmdBytes, err := os.ReadFile(filepath.Join(p, "cmdline"))
		if err != nil || len(cmdBytes) == 0 {
			continue
		}
		cmdline := string(bytes.ReplaceAll(cmdBytes, []byte{0}, []byte{' '}))

		comm := ""
		if commBytes, err := os.ReadFile(filepath.Join(p, "comm")); err == nil {
			comm = strings.TrimSpace(string(commBytes))
		}

		if strings.Contains(cmdline, "steamapps/common/") {
			idx := strings.Index(cmdline, "steamapps/common/")
			sub := strings.Split(cmdline[idx+len("steamapps/common/"):], "/")[0]
			if sub != "" && !strings.HasPrefix(sub, "SteamLinuxRuntime") {
				return "Steam: " + sub, &pid
			}
		} else if strings.Contains(comm, "gamescope") || strings.Contains(cmdline, "gamescope") {
			return "Gamescope Session", &pid
		} else if strings.Contains(strings.ToLower(comm), "wine") || strings.Contains(strings.ToLower(cmdline), "proton") {
			for _, part := range strings.Fields(cmdline) {
				low := strings.ToLower(part)
				if strings.HasSuffix(low, ".exe") && !strings.Contains(low, "windows") && !strings.Contains(low, "steam") {
					return "Proton: " + filepath.Base(part), &pid
				}
			}
			if !strings.Contains(comm, "steamwebhelper") {
				return "Proton/Wine Game", &pid
			}
		} else {
			lowComm := strings.ToLower(comm)
			switch lowComm {
			case "cs2", "dota2", "hl2_linux", "valheim.x86_64", "overwatch", "cyberpunk2077", "rpcs3", "yuzu", "ryujinx":
				return comm, &pid
			}
		}
	}
	return "Desktop / Idle", nil
}

func (sc *SensorCollector) GetGamingStats(cpu CPUStats, gpus []GPUStats, wallW float64, costRate float64) GamingStats {
	gst := GamingStats{}
	gameName, gamePid := sc.DetectActiveGame()
	gst.ActiveGame = gameName
	gst.ActiveGamePid = gamePid

	gpuOver90 := false
	for _, g := range gpus {
		if g.PowerW > 90.0 {
			gpuOver90 = true
			break
		}
	}
	gst.IsGaming = (gameName != "Desktop / Idle") || gpuOver90

	// GPU sysfs hwmon
	hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range hwmons {
		if nameB, err := os.ReadFile(filepath.Join(h, "name")); err == nil {
			if strings.Contains(string(nameB), "amdgpu") {
				for i := 1; i <= 3; i++ {
					lblFile := filepath.Join(h, fmt.Sprintf("temp%d_label", i))
					inpFile := filepath.Join(h, fmt.Sprintf("temp%d_input", i))
					if inpB, err := os.ReadFile(inpFile); err == nil {
						if tval, err := strconv.ParseFloat(strings.TrimSpace(string(inpB)), 64); err == nil {
							tC := tval / 1000.0
							lbl := fmt.Sprintf("temp%d", i)
							if lblB, err := os.ReadFile(lblFile); err == nil {
								lbl = strings.TrimSpace(string(lblB))
							}
							if strings.Contains(lbl, "edge") {
								gst.TempEdgeC = &tC
							} else if strings.Contains(lbl, "junction") || strings.Contains(lbl, "hotspot") {
								gst.TempJunctionC = &tC
							} else if strings.Contains(lbl, "mem") {
								gst.TempMemC = &tC
							}
						}
					}
				}
				if f1B, err := os.ReadFile(filepath.Join(h, "freq1_input")); err == nil {
					if fval, err := strconv.ParseFloat(strings.TrimSpace(string(f1B)), 64); err == nil {
						gst.SclkMhz = fval / 1000000.0
					}
				}
				if f2B, err := os.ReadFile(filepath.Join(h, "freq2_input")); err == nil {
					if fval, err := strconv.ParseFloat(strings.TrimSpace(string(f2B)), 64); err == nil {
						gst.MclkMhz = fval / 1000000.0
					}
				}
				if in0B, err := os.ReadFile(filepath.Join(h, "in0_input")); err == nil {
					if ival, err := strconv.ParseFloat(strings.TrimSpace(string(in0B)), 64); err == nil {
						gst.VoltageMv = ival
					}
				}
				break
			}
		}
	}

	// DRM busy percentages
	drmDevices, _ := filepath.Glob("/sys/class/drm/card*/device")
	for _, d := range drmDevices {
		if b, err := os.ReadFile(filepath.Join(d, "gpu_busy_percent")); err == nil {
			if val, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
				gst.GpuBusyPct = val
				if memB, err := os.ReadFile(filepath.Join(d, "mem_busy_percent")); err == nil {
					if mv, err := strconv.ParseFloat(strings.TrimSpace(string(memB)), 64); err == nil {
						gst.MemBusyPct = mv
					}
				}
				break
			}
		}
	}

	if len(cpu.CoreFreqs) > 0 {
		maxF := cpu.CoreFreqs[0]
		for _, f := range cpu.CoreFreqs {
			if f > maxF {
				maxF = f
			}
		}
		gst.CpuPeakBoostMhz = maxF
	} else {
		gst.CpuPeakBoostMhz = cpu.AvgFreqMhz
	}

	if len(cpu.CoreLoads) > 0 {
		maxL := cpu.CoreLoads[0]
		for _, l := range cpu.CoreLoads {
			if l > maxL {
				maxL = l
			}
		}
		gst.CpuPeakCoreLoadPct = maxL
	} else {
		gst.CpuPeakCoreLoadPct = cpu.TotalLoad
	}

	// Bottleneck analyzer
	if gst.GpuBusyPct >= 80.0 && gst.GpuBusyPct >= gst.CpuPeakCoreLoadPct+10.0 {
		gst.BottleneckStatus = fmt.Sprintf("GPU Bound (%d%% 3D Load)", int(gst.GpuBusyPct))
	} else if gst.CpuPeakCoreLoadPct >= 80.0 && gst.CpuPeakCoreLoadPct >= gst.GpuBusyPct+10.0 {
		gst.BottleneckStatus = fmt.Sprintf("CPU Bound (%d%% Single-Core)", int(gst.CpuPeakCoreLoadPct))
	} else if gst.GpuBusyPct >= 35.0 || gst.CpuPeakCoreLoadPct >= 35.0 {
		gst.BottleneckStatus = fmt.Sprintf("Balanced (GPU %d%% / CPU %d%%)", int(gst.GpuBusyPct), int(gst.CpuPeakCoreLoadPct))
	} else {
		gst.BottleneckStatus = "Idle / Desktop Load"
	}

	gst.GamingCostPerHourZar = (wallW / 1000.0) * costRate
	return gst
}

type procEntry struct {
	pid             int
	comm            string
	username        string
	rawCpuUsageSec  float64
	dtProc          float64
	rssMb           float64
}

func (sc *SensorCollector) GetTopProcesses(cpuPowerW float64, gpuPowerW float64) []ProcessPower {
	now := float64(time.Now().UnixNano()) / 1e9
	newProcTimes := make(map[int][2]float64)
	var procEntries []procEntry
	totalDeltaCpu := 0.0

	pageSize := float64(os.Getpagesize())
	hz := 100.0 // Standard Linux default clock ticks

	procDir, err := os.Open("/proc")
	if err != nil {
		return nil
	}
	names, err := procDir.Readdirnames(-1)
	procDir.Close()
	if err != nil {
		return nil
	}

	buf := make([]byte, 2048)
	for _, pidStr := range names {
		if len(pidStr) == 0 || pidStr[0] < '0' || pidStr[0] > '9' {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		statPath := "/proc/" + pidStr + "/stat"
		f, err := os.Open(statPath)
		if err != nil {
			continue
		}
		n, err := f.Read(buf)
		f.Close()
		if err != nil || n <= 0 {
			continue
		}
		content := buf[:n]

		lParen := bytes.IndexByte(content, '(')
		rParen := bytes.LastIndexByte(content, ')')
		if lParen == -1 || rParen == -1 || rParen <= lParen {
			continue
		}
		comm := string(content[lParen+1 : rParen])
		rest := bytes.Fields(content[rParen+2:])
		if len(rest) < 22 {
			continue
		}

		utime, _ := strconv.ParseInt(string(rest[11]), 10, 64)
		stime, _ := strconv.ParseInt(string(rest[12]), 10, 64)
		totTicks := float64(utime + stime)

		rssPages, _ := strconv.ParseInt(string(rest[21]), 10, 64)
		rssMb := (float64(rssPages) * pageSize) / (1024.0 * 1024.0)

		username := "user"
		uid := 1000
		if name, ok := sc.userCache[uid]; ok {
			username = name
			if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
				sc.userCache[uid] = u.Username
				username = u.Username
			}
		}

		newProcTimes[pid] = [2]float64{totTicks, now}

		if prev, ok := sc.lastProcTimes[pid]; ok {
			prevTicks := prev[0]
			prevTime := prev[1]
			dt := now - prevTime
			if dt > 0.1 {
				dTicks := math.Max(0.0, totTicks-prevTicks)
				cpuUsageSec := dTicks / hz
				normCpuPct := (cpuUsageSec / (dt * math.Max(1.0, float64(sc.cpuThreads)))) * 100.0
				if normCpuPct > 0.05 || rssMb > 50.0 {
					procEntries = append(procEntries, procEntry{
						pid:            pid,
						comm:           comm,
						username:       username,
						rawCpuUsageSec: cpuUsageSec,
						dtProc:         dt,
						rssMb:          rssMb,
					})
					totalDeltaCpu += cpuUsageSec
				}
			}
		}
	}
	sc.lastProcTimes = newProcTimes

	totalRamMb := 16384.0
	if f, err := os.Open("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					if val, err := strconv.ParseFloat(parts[1], 64); err == nil {
						totalRamMb = val / 1024.0
					}
				}
				break
			}
		}
		f.Close()
	}

	var allProcs []ProcessPower
	for _, pe := range procEntries {
		cpuPct := math.Min(100.0, (pe.rawCpuUsageSec/(pe.dtProc*math.Max(1.0, float64(sc.cpuThreads))))*100.0)
		memPct := math.Min(100.0, (pe.rssMb/totalRamMb)*100.0)

		var procCpuW float64
		if totalDeltaCpu > 0.01 {
			procCpuW = (pe.rawCpuUsageSec / math.Max(0.001, totalDeltaCpu)) * cpuPowerW
		} else {
			procCpuW = (cpuPct / 100.0) * cpuPowerW
		}
		procMemW := (memPct / 100.0) * 4.0
		estWatts := math.Max(0.01, procCpuW+procMemW)

		allProcs = append(allProcs, ProcessPower{
			Pid:            pe.pid,
			Name:           pe.comm,
			User:           pe.username,
			CpuPercent:     cpuPct,
			MemPercent:     memPct,
			EstimatedWatts: estWatts,
		})
	}

	var heavyProcs []ProcessPower
	var otherProcs []ProcessPower
	for _, p := range allProcs {
		if p.EstimatedWatts >= 5.0 {
			heavyProcs = append(heavyProcs, p)
		} else {
			otherProcs = append(otherProcs, p)
		}
	}

	sort.Slice(heavyProcs, func(i, j int) bool {
		return heavyProcs[i].EstimatedWatts > heavyProcs[j].EstimatedWatts
	})

	if len(otherProcs) > 0 {
		otherCpu := 0.0
		otherMem := 0.0
		otherPower := 0.0
		for _, p := range otherProcs {
			otherCpu += p.CpuPercent
			otherMem += p.MemPercent
			otherPower += p.EstimatedWatts
		}
		summaryProc := ProcessPower{
			Pid:            0,
			Name:           fmt.Sprintf("Other (<5W, %d procs)", len(otherProcs)),
			User:           "system",
			CpuPercent:     math.Min(100.0, otherCpu),
			MemPercent:     math.Min(100.0, otherMem),
			EstimatedWatts: otherPower,
		}
		heavyProcs = append(heavyProcs, summaryProc)
	}

	return heavyProcs
}

func (sc *SensorCollector) Sample(psuProfile string) SystemPowerSnapshot {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	now := float64(time.Now().UnixNano()) / 1e9
	dt := math.Max(0.1, now-sc.lastSampleTime)
	sc.lastSampleTime = now

	cpu := sc.GetCPUStats(dt)
	gpus := sc.GetGPUStats()
	battery := sc.GetBatteryStats()
	other := sc.GetOtherStats(dt)
	bandwidth := sc.GetBandwidthStats(dt)

	gpuSum := 0.0
	for _, g := range gpus {
		gpuSum += g.PowerW
	}
	measuredDc := cpu.PackagePowerW + gpuSum
	totalEstimatedDc := measuredDc + other.RamPowerW + other.DiskPowerW + other.MotherboardPowerW

	var wallAc, efficiency float64
	if battery.Present && battery.Status == "Discharging" && battery.PowerW > 0 {
		wallAc = battery.PowerW
		efficiency = 1.00
	} else {
		eff, ok := PSUEfficiencyProfiles[psuProfile]
		if !ok {
			eff = 0.90
		}
		efficiency = eff
		wallAc = totalEstimatedDc / efficiency
	}

	topProcs := sc.GetTopProcesses(cpu.PackagePowerW, gpuSum)
	gaming := sc.GetGamingStats(cpu, gpus, wallAc, 3.50)

	return SystemPowerSnapshot{
		Timestamp:         now,
		CPU:               cpu,
		GPUs:              gpus,
		Battery:           battery,
		Other:             other,
		Bandwidth:         bandwidth,
		Gaming:            gaming,
		MeasuredDcW:       measuredDc,
		TotalEstimatedDcW: totalEstimatedDc,
		WallAcW:           wallAc,
		PsuEfficiency:     efficiency,
		PsuProfile:        psuProfile,
		TopProcesses:      topProcs,
	}
}
