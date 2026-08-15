package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
	"powertui/pkg/sensors"
	"powertui/pkg/weather"
)

// ANSI Color Helpers
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Italic      = "\033[3m"
	Underline   = "\033[4m"
	Inverse     = "\033[7m"
	ClearScreen = "\033[2J"
	HideCursor  = "\033[?25l"
	ShowCursor  = "\033[?25h"
	AltScreenOn = "\033[?1049h"
	AltScreenOff= "\033[?1049l"

	// Standard Colors
	FgBlack   = "\033[30m"
	FgRed     = "\033[31m"
	FgGreen   = "\033[32m"
	FgYellow  = "\033[33m"
	FgBlue    = "\033[34m"
	FgMagenta = "\033[35m"
	FgCyan    = "\033[36m"
	FgWhite   = "\033[37m"

	// Bright Colors
	FgHiBlack   = "\033[90m"
	FgHiRed     = "\033[91m"
	FgHiGreen   = "\033[92m"
	FgHiYellow  = "\033[93m"
	FgHiBlue    = "\033[94m"
	FgHiMagenta = "\033[95m"
	FgHiCyan    = "\033[96m"
	FgHiWhite   = "\033[97m"

	// Backgrounds
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
)

type Theme struct {
	Name      string
	Header    string
	Accent    string
	Warning   string
	Danger    string
	Dim       string
	Bar       string
	Graph     string
	ModalBg   string
}

var Themes = []Theme{
	{
		Name:    "Cyber Neon",
		Header:  FgHiCyan,
		Accent:  FgHiGreen,
		Warning: FgHiYellow,
		Danger:  FgHiRed,
		Dim:     FgHiBlack,
		Bar:     FgHiCyan,
		Graph:   FgHiGreen,
		ModalBg: BgBlack + FgHiCyan,
	},
	{
		Name:    "Matrix Green",
		Header:  FgHiGreen,
		Accent:  FgGreen,
		Warning: FgHiYellow,
		Danger:  FgHiRed,
		Dim:     FgHiBlack,
		Bar:     FgHiGreen,
		Graph:   FgGreen,
		ModalBg: BgBlack + FgHiGreen,
	},
	{
		Name:    "Sunset Amber",
		Header:  FgHiYellow,
		Accent:  FgYellow,
		Warning: FgHiMagenta,
		Danger:  FgHiRed,
		Dim:     FgHiBlack,
		Bar:     FgHiYellow,
		Graph:   FgYellow,
		ModalBg: BgBlack + FgHiYellow,
	},
	{
		Name:    "Arctic Blue",
		Header:  FgHiBlue,
		Accent:  FgHiCyan,
		Warning: FgHiYellow,
		Danger:  FgHiRed,
		Dim:     FgHiBlack,
		Bar:     FgHiCyan,
		Graph:   FgHiBlue,
		ModalBg: BgBlack + FgHiBlue,
	},
	{
		Name:    "Monochrome",
		Header:  FgHiWhite,
		Accent:  FgWhite,
		Warning: FgHiWhite,
		Danger:  FgHiWhite,
		Dim:     FgHiBlack,
		Bar:     FgWhite,
		Graph:   FgHiWhite,
		ModalBg: BgBlack + FgWhite,
	},
}

type HeavyProcRecord struct {
	Name  string
	Pid   int
	User  string
	Power float64
	CPU   float64
	Mem   float64
}

type App struct {
	mu               sync.Mutex
	collector        *sensors.SensorCollector
	weatherService   *weather.WeatherService
	isPaused         bool
	refreshInterval  time.Duration
	rateOptions      []float64
	rateIdx          int
	psuProfiles      []string
	psuIdx           int
	psuCapacityW     float64
	themeIdx         int
	graphMode        string // "braille" or "blocks"
	procSort         string // "power", "cpu", "mem", "pid"
	costPerKwh       float64
	co2PerKwhG       float64

	showHelp         bool
	showCostModal    bool
	showWeatherModal bool
	costInputBuf     string

	historyMax       int
	historyWallW     []float64
	historyCpuW      []float64
	historyGpuW      []float64

	sessionStart     time.Time
	lastUpdate       time.Time
	totalJoules      float64
	peakPowerW       float64
	minPowerW        float64
	sampleCount      int64
	sumPowerW        float64
	sumCpuW          float64
	sumGpuW          float64
	sessionTopProcs  map[string]HeavyProcRecord

	peakGpuSclkMhz   float64
	peakGpuHotspotC  float64
	peakGpuMemTempC  float64
	peakCpuBoostMhz  float64

	currentSnapshot  sensors.SystemPowerSnapshot
	termWidth        int
	termHeight       int
	oldState         *term.State
}

func NewApp() *App {
	psuList := []string{"80+ Gold", "80+ Titanium", "80+ Platinum", "80+ Silver", "80+ Bronze", "Standard", "Raw DC (100%)"}
	app := &App{
		collector:       sensors.NewSensorCollector(),
		weatherService:  weather.NewWeatherService(),
		refreshInterval: 1000 * time.Millisecond,
		rateOptions:     []float64{0.2, 0.5, 1.0, 2.0, 5.0},
		rateIdx:         2,
		psuProfiles:     psuList,
		psuIdx:          0,
		psuCapacityW:    750.0,
		themeIdx:        0,
		graphMode:       "braille",
		procSort:        "power",
		costPerKwh:      3.50,
		co2PerKwhG:      920.0,
		historyMax:      300,
		sessionStart:    time.Now(),
		lastUpdate:      time.Now(),
		minPowerW:       math.Inf(1),
		sessionTopProcs: make(map[string]HeavyProcRecord),
	}
	return app
}

func (a *App) CurrentTheme() Theme {
	return Themes[a.themeIdx]
}

func (a *App) ResetStats() {
	a.sessionStart = time.Now()
	a.totalJoules = 0
	a.peakPowerW = 0
	a.minPowerW = math.Inf(1)
	a.sampleCount = 0
	a.sumPowerW = 0
	a.sumCpuW = 0
	a.sumGpuW = 0
	a.peakGpuSclkMhz = 0
	a.peakGpuHotspotC = 0
	a.peakGpuMemTempC = 0
	a.peakCpuBoostMhz = 0
	a.sessionTopProcs = make(map[string]HeavyProcRecord)
	a.historyWallW = nil
	a.historyCpuW = nil
	a.historyGpuW = nil
	a.weatherService.RefreshNow()
}

func (a *App) UpdateData() {
	profile := a.psuProfiles[a.psuIdx]
	now := time.Now()
	dt := now.Sub(a.lastUpdate).Seconds()
	a.lastUpdate = now

	snap := a.collector.Sample(profile)
	a.currentSnapshot = snap

	wallW := snap.WallAcW
	cpuW := snap.CPU.PackagePowerW
	gpuW := 0.0
	for _, g := range snap.GPUs {
		gpuW += g.PowerW
	}

	a.historyWallW = append(a.historyWallW, wallW)
	if len(a.historyWallW) > a.historyMax {
		a.historyWallW = a.historyWallW[1:]
	}
	a.historyCpuW = append(a.historyCpuW, cpuW)
	if len(a.historyCpuW) > a.historyMax {
		a.historyCpuW = a.historyCpuW[1:]
	}
	a.historyGpuW = append(a.historyGpuW, gpuW)
	if len(a.historyGpuW) > a.historyMax {
		a.historyGpuW = a.historyGpuW[1:]
	}

	if dt > 0 && dt < 10.0 {
		a.totalJoules += wallW * dt
	}

	a.sampleCount++
	a.sumPowerW += wallW
	a.sumCpuW += cpuW
	a.sumGpuW += gpuW
	if wallW > a.peakPowerW {
		a.peakPowerW = wallW
	}
	if wallW < a.minPowerW {
		a.minPowerW = wallW
	}

	gm := snap.Gaming
	if gm.SclkMhz > a.peakGpuSclkMhz {
		a.peakGpuSclkMhz = gm.SclkMhz
	}
	if gm.TempJunctionC != nil && *gm.TempJunctionC > a.peakGpuHotspotC {
		a.peakGpuHotspotC = *gm.TempJunctionC
	}
	if gm.TempMemC != nil && *gm.TempMemC > a.peakGpuMemTempC {
		a.peakGpuMemTempC = *gm.TempMemC
	}
	if gm.CpuPeakBoostMhz > a.peakCpuBoostMhz {
		a.peakCpuBoostMhz = gm.CpuPeakBoostMhz
	}

	for _, p := range snap.TopProcesses {
		if p.Pid != 0 && p.EstimatedWatts >= 5.0 {
			key := fmt.Sprintf("%s_%d", p.Name, p.Pid)
			if prev, ok := a.sessionTopProcs[key]; !ok || p.EstimatedWatts > prev.Power {
				a.sessionTopProcs[key] = HeavyProcRecord{
					Name:  p.Name,
					Pid:   p.Pid,
					User:  p.User,
					Power: p.EstimatedWatts,
					CPU:   p.CpuPercent,
					Mem:   p.MemPercent,
				}
			}
		}
	}
}

func formatBytes(b float64) string {
	if b < 1024 {
		return fmt.Sprintf("%.0f B", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", b/1024)
	} else if b < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", b/(1024*1024))
	} else if b < 1024*1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", b/(1024*1024*1024))
	}
	return fmt.Sprintf("%.3f TB", b/(1024*1024*1024*1024))
}

func makeBar(val float64, maxVal float64, width int, fillChar string) string {
	if width <= 0 {
		return ""
	}
	ratio := math.Max(0.0, math.Min(1.0, val/math.Max(1.0, maxVal)))
	filled := int(math.Round(ratio * float64(width)))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return strings.Repeat(fillChar, filled) + strings.Repeat("░", empty)
}

func (a *App) RenderBrailleGraph(values []float64, maxVal float64, minVal float64, width int, height int) []string {
	if len(values) == 0 || width <= 0 || height <= 0 {
		var blank []string
		for i := 0; i < height; i++ {
			blank = append(blank, strings.Repeat(" ", width))
		}
		return blank
	}

	res := make([]string, height)
	valRange := math.Max(1.0, maxVal-minVal)

	// Sub-grid 2 columns x 4 rows per Braille character
	pixelWidth := width * 2
	pixelHeight := height * 4

	padded := make([]float64, pixelWidth)
	copyStart := pixelWidth - len(values)
	if copyStart >= 0 {
		for i := 0; i < copyStart; i++ {
			padded[i] = values[0]
		}
		copy(padded[copyStart:], values)
	} else {
		copy(padded, values[len(values)-pixelWidth:])
	}

	normY := make([]int, pixelWidth)
	for i, v := range padded {
		norm := math.Max(0.0, math.Min(1.0, (v-minVal)/valRange))
		y := int(math.Round(norm * float64(pixelHeight-1)))
		normY[i] = y
	}

	brailleOffsets := [4][2]rune{
		{0x01, 0x08},
		{0x02, 0x10},
		{0x04, 0x20},
		{0x40, 0x80},
	}

	for r := 0; r < height; r++ {
		charRow := height - 1 - r
		var sb strings.Builder
		for c := 0; c < width; c++ {
			charCol := c
			code := rune(0x2800)
			for dy := 0; dy < 4; dy++ {
				py := charRow*4 + dy
				for dx := 0; dx < 2; dx++ {
					px := charCol*2 + dx
					if px < len(normY) {
						targetY := normY[px]
						if targetY >= py {
							code |= brailleOffsets[3-dy][dx]
						}
					}
				}
			}
			sb.WriteRune(code)
		}
		res[r] = sb.String()
	}
	return res
}

func (a *App) RenderBlocksGraph(values []float64, maxVal float64, minVal float64, width int, height int) []string {
	res := make([]string, height)
	if len(values) == 0 || width <= 0 || height <= 0 {
		for i := 0; i < height; i++ {
			res[i] = strings.Repeat(" ", width)
		}
		return res
	}

	valRange := math.Max(1.0, maxVal-minVal)
	padded := make([]float64, width)
	copyStart := width - len(values)
	if copyStart >= 0 {
		for i := 0; i < copyStart; i++ {
			padded[i] = values[0]
		}
		copy(padded[copyStart:], values)
	} else {
		copy(padded, values[len(values)-width:])
	}

	levels := []rune{' ', ' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	for r := 0; r < height; r++ {
		rowIdx := height - 1 - r
		var sb strings.Builder
		for c := 0; c < width; c++ {
			norm := math.Max(0.0, math.Min(1.0, (padded[c]-minVal)/valRange))
			totalLevels := float64(height * 8)
			curLevel := norm * totalLevels
			rowMin := float64(rowIdx * 8)
			rowMax := float64((rowIdx + 1) * 8)

			if curLevel >= rowMax {
				sb.WriteRune('█')
			} else if curLevel <= rowMin {
				sb.WriteRune(' ')
			} else {
				sub := int(math.Round(curLevel - rowMin))
				if sub < 0 {
					sub = 0
				}
				if sub > 8 {
					sub = 8
				}
				sb.WriteRune(levels[sub])
			}
		}
		res[r] = sb.String()
	}
	return res
}

func (a *App) DrawScreen() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	var buf strings.Builder
	th := a.CurrentTheme()
	snap := a.currentSnapshot
	w := a.termWidth
	h := a.termHeight

	if w < 70 || h < 20 {
		buf.WriteString("\033[H\033[2J")
		buf.WriteString(fmt.Sprintf("%s%s[ Terminal Window Too Small: %dx%d ]%s\n", Bold, FgHiYellow, w, h, Reset))
		buf.WriteString("Please expand your terminal window (minimum 80x24 recommended).\n")
		return buf.String()
	}

	// Double buffering move cursor to top-left
	buf.WriteString("\033[H")

	// 1. Header Bar
	headerTitle := fmt.Sprintf(" ⚡ POWERTUI (Go) │ %s │ %s", snap.CPU.Model, snap.PsuProfile)
	if len(snap.GPUs) > 0 {
		headerTitle += fmt.Sprintf(" │ %s", snap.GPUs[0].Name)
	}
	pauseTag := ""
	if a.isPaused {
		pauseTag = " [PAUSED]"
	}
	rightTag := fmt.Sprintf("Theme: %s │ Refresh: %.1fs%s │ [H]elp [Q]uit ", th.Name, a.refreshInterval.Seconds(), pauseTag)
	spacesH := w - len(headerTitle) - len(rightTag)
	if spacesH < 1 {
		spacesH = 1
	}
	headerLine := headerTitle + strings.Repeat(" ", spacesH) + rightTag
	if len(headerLine) > w {
		headerLine = headerLine[:w]
	}
	buf.WriteString(fmt.Sprintf("%s%s%s%s\n", Inverse, th.Header, headerLine, Reset))

	// 2. Power Gauges & Stats Line
	wallW := snap.WallAcW
	cpuW := snap.CPU.PackagePowerW
	gpuW := 0.0
	for _, g := range snap.GPUs {
		gpuW += g.PowerW
	}
	boardW := snap.Other.MotherboardPowerW
	ramW := snap.Other.RamPowerW
	diskW := snap.Other.DiskPowerW
	dcSensorsW := snap.MeasuredDcW

	powerColor := th.Accent
	if wallW > a.psuCapacityW*0.80 {
		powerColor = th.Danger
	} else if wallW > a.psuCapacityW*0.50 {
		powerColor = th.Warning
	}

	buf.WriteString(fmt.Sprintf(" %s%sESTIMATED WALL POWER:%s %s%6.1f W%s │ %sDC SENSORS:%s %5.1f W │ %sCPU:%s %5.1f W │ %sGPU:%s %5.1f W │ %sRAM:%s %4.1f W │ %sSSD:%s %4.1f W │ %sBOARD:%s %4.1f W\n",
		Bold, th.Header, Reset, Bold+powerColor, wallW, Reset,
		th.Accent, Reset, dcSensorsW,
		th.Accent, Reset, cpuW,
		th.Accent, Reset, gpuW,
		th.Dim, Reset, ramW,
		th.Dim, Reset, diskW,
		th.Dim, Reset, boardW,
	))

	// 3. Wall PSU Power Load Gauge
	gaugeW := w - 24
	if gaugeW < 10 {
		gaugeW = 10
	}
	barStr := makeBar(wallW, a.psuCapacityW, gaugeW, "█")
	buf.WriteString(fmt.Sprintf(" PSU Load [%s]: %s%s%s %s%5.1f%%%s (%4.0fW Cap)\n",
		snap.PsuProfile, th.Bar, barStr, Reset, Bold+powerColor, (wallW/a.psuCapacityW)*100.0, Reset, a.psuCapacityW))

	// 4. Power Graph (Braille or Blocks)
	graphH := 6
	graphW := w - 16
	if graphW < 20 {
		graphW = 20
	}

	maxGraphVal := 50.0
	minGraphVal := 0.0
	for _, v := range a.historyWallW {
		if v > maxGraphVal {
			maxGraphVal = v
		}
	}
	maxGraphVal = math.Ceil(maxGraphVal/20.0) * 20.0

	var graphLines []string
	if a.graphMode == "blocks" {
		graphLines = a.RenderBlocksGraph(a.historyWallW, maxGraphVal, minGraphVal, graphW, graphH)
	} else {
		graphLines = a.RenderBrailleGraph(a.historyWallW, maxGraphVal, minGraphVal, graphW, graphH)
	}

	for idx, gLine := range graphLines {
		yVal := maxGraphVal - (float64(idx)/float64(graphH))*(maxGraphVal-minGraphVal)
		buf.WriteString(fmt.Sprintf("%s%6.0f W ┤%s%s%s%s\n", th.Dim, yVal, Reset, th.Graph, gLine, Reset))
	}
	buf.WriteString(fmt.Sprintf("%s%s └%s%s\n", th.Dim, "       ", strings.Repeat("─", graphW), Reset))

	// 5. Weather & Cycling Forecast Box
	wSnap := a.weatherService.GetSnapshot()
	buf.WriteString(fmt.Sprintf("%s%s┌─ 🌤️ WEATHER & CYCLING FORECAST [Key 'W' for full outlook] ──────────────────────────┐%s\n", Bold, th.Header, Reset))
	
	// Weather line 1
	wRatingColor := FgHiGreen
	if wSnap.CurrentCyclingRating == "FAIR" {
		wRatingColor = FgHiYellow
	} else if wSnap.CurrentCyclingRating == "POOR" {
		wRatingColor = FgHiRed
	}
	locWeather := fmt.Sprintf(" 📍 %s │ %s %s │ %.1f°C (Feels %.1f°C) │ 💨 %.0f km/h │ 🚲 [%s%s%s]",
		wSnap.City, wSnap.CurrentWeatherIcon, wSnap.CurrentWeatherDesc, wSnap.CurrentTempC, wSnap.CurrentApparentC, wSnap.CurrentWindKmh,
		Bold+wRatingColor, wSnap.CurrentCyclingRating, Reset,
	)
	buf.WriteString(locWeather + "\n")

	// Weather line 2: Next 3 hours
	if len(wSnap.Next3Hours) > 0 {
		var hParts []string
		for _, hf := range wSnap.Next3Hours {
			hIcon := "🟢"
			if hf.CyclingRating == "FAIR" {
				hIcon = "🟡"
			} else if hf.CyclingRating == "POOR" {
				hIcon = "🔴"
			}
			hParts = append(hParts, fmt.Sprintf("%s: %.0f°C 💨%.0fk 💧%d%% %s%s", hf.HourLabel, hf.TempC, hf.WindSpeedKmh, hf.PrecipProbPct, hIcon, hf.CyclingRating))
		}
		buf.WriteString(fmt.Sprintf(" %sNext 3h:%s %s\n", th.Accent, Reset, strings.Join(hParts, " │ ")))
	}

	// Weather line 3: Tomorrow Morning 5-10 AM
	if m := wSnap.TomorrowMorning5to10; m != nil {
		mIcon := "🟢"
		mColor := FgHiGreen
		if m.OverallRating == "FAIR" {
			mIcon = "🟡"
			mColor = FgHiYellow
		} else if m.OverallRating == "POOR" {
			mIcon = "🔴"
			mColor = FgHiRed
		}
		buf.WriteString(fmt.Sprintf(" %sTomorrow (05:00-10:00):%s %s %s%s (%d/100)%s — %s\n",
			th.Accent, Reset, mIcon, Bold+mColor, m.OverallRating, m.OverallScore, Reset, m.Summary))
	}
	buf.WriteString(fmt.Sprintf("%s%s└────────────────────────────────────────────────────────────────────────────────────────┘%s\n", th.Header, Reset, Reset))

	// 6. Cumulative Energy & Analytics Line
	kwh := (a.totalJoules / 3600.0) / 1000.0
	costZar := kwh * a.costPerKwh
	co2Grams := kwh * a.co2PerKwhG
	avgPower := 0.0
	if a.sampleCount > 0 {
		avgPower = a.sumPowerW / float64(a.sampleCount)
	}
	minP := a.minPowerW
	if math.IsInf(minP, 1) {
		minP = 0
	}

	buf.WriteString(fmt.Sprintf(" %sANALYTICS:%s Energy: %s%.3f kWh%s (%.1f kJ) │ Cost: %sR %.2f%s (@ R%.2f/kWh) │ CO₂: %s%.1f g%s │ Avg: %4.1f W │ Peak: %4.1f W │ Min: %4.1f W\n",
		Bold+th.Header, Reset,
		th.Accent, kwh, Reset, a.totalJoules/1000.0,
		Bold+th.Accent, costZar, Reset, a.costPerKwh,
		th.Dim, co2Grams, Reset,
		avgPower, a.peakPowerW, minP,
	))

	// 7. Active Processes Table
	buf.WriteString(fmt.Sprintf("\n%s%s  PID     COMMAND              USER         CPU %%   MEM %%     EST POWER%s\n", Bold, th.Header, Reset))
	buf.WriteString(fmt.Sprintf("%s  ─────── ──────────────────── ───────── ──────── ──────── ─────────────%s\n", th.Dim, Reset))

	for i, p := range snap.TopProcesses {
		if i >= 6 {
			break
		}
		pName := p.Name
		if len(pName) > 20 {
			pName = pName[:20]
		}
		pPidStr := fmt.Sprintf("%d", p.Pid)
		if p.Pid == 0 {
			pPidStr = "-"
		}
		pPowerColor := Reset
		if p.EstimatedWatts >= 15.0 {
			pPowerColor = Bold + th.Danger
		} else if p.EstimatedWatts >= 5.0 {
			pPowerColor = th.Warning
		}

		buf.WriteString(fmt.Sprintf("  %-7s %-20s %-9s %7.1f%% %7.1f%% %s%10.2f W%s\n",
			pPidStr, pName, p.User, p.CpuPercent, p.MemPercent, pPowerColor, p.EstimatedWatts, Reset))
	}

	// 8. Gaming & Telemetry Box
	gm := snap.Gaming
	gameStatus := gm.ActiveGame
	if gm.IsGaming {
		gameStatus = fmt.Sprintf("🎮 ACTIVE: %s", gm.ActiveGame)
	}
	buf.WriteString(fmt.Sprintf("\n %sGAMING & TELEMETRY:%s %s │ %sBottleneck:%s %s │ %sGPU SCLK:%s %.0f MHz │ %sHotspot:%s %.0f°C │ %sCost/Hr:%s R %.2f\n",
		Bold+th.Header, Reset, gameStatus,
		th.Accent, Reset, gm.BottleneckStatus,
		th.Dim, Reset, gm.SclkMhz,
		th.Dim, Reset, a.peakGpuHotspotC,
		th.Accent, Reset, gm.GamingCostPerHourZar,
	))

	// 9. Status & Controls Footer
	buf.WriteString(fmt.Sprintf("%s%s [Q] Quit  [P] Pause  [R] Reset/Refresh  [W] Weather Window  [C] Theme  [E] PSU Profile  [G] Graph  [K] Rates %s",
		Inverse, th.Header, Reset))

	// Overlay Modals
	if a.showWeatherModal {
		a.drawWeatherModalOverlay(&buf)
	} else if a.showHelp {
		a.drawHelpModalOverlay(&buf)
	}

	return buf.String()
}

func (a *App) drawWeatherModalOverlay(buf *strings.Builder) {
	th := a.CurrentTheme()
	wSnap := a.weatherService.GetSnapshot()

	modalLines := []string{
		fmt.Sprintf("╔════════════════════════════════ LOCAL WEATHER & CYCLING INTELLIGENCE ════════════════════════════════╗"),
		fmt.Sprintf("║ 📍 Location: %s, %s (%.2f°, %.2f°) │ Timezone: %s", wSnap.City, wSnap.Country, wSnap.Lat, wSnap.Lon, wSnap.Timezone),
		fmt.Sprintf("║ Weather: %s %s │ Temp: %.1f°C (Feels %.1f°C) │ Rain: %.1fmm │ Wind: %.0f km/h (Gusts: %.0f) │ Humidity: %d%%",
			wSnap.CurrentWeatherIcon, wSnap.CurrentWeatherDesc, wSnap.CurrentTempC, wSnap.CurrentApparentC, wSnap.CurrentPrecipMM, wSnap.CurrentWindKmh, wSnap.CurrentWindGustsKmh, wSnap.CurrentHumidity),
		fmt.Sprintf("║ Ride Rating: [%s] %s", wSnap.CurrentCyclingRating, wSnap.CurrentCyclingVerdict),
		fmt.Sprintf("╠───────────────────────────────────────────────────────────────────────────────────────────────────────╣"),
		fmt.Sprintf("║ 🕒 NEXT 3 HOURS (HOUR-BY-HOUR CYCLING PREDICTION):"),
		fmt.Sprintf("║   Hour    Condition          Temp (Feels)    Precip          Wind / Gusts       Ride Verdict"),
	}

	for _, h := range wSnap.Next3Hours {
		rIcon := "🟢"
		if h.CyclingRating == "FAIR" {
			rIcon = "🟡"
		} else if h.CyclingRating == "POOR" {
			rIcon = "🔴"
		}
		tStr := fmt.Sprintf("%.1f°C (%.1f°C)", h.TempC, h.ApparentTempC)
		pStr := fmt.Sprintf("%d%% (%.1fmm)", h.PrecipProbPct, h.PrecipMM)
		wStr := fmt.Sprintf("%.0f / %.0f km/h", h.WindSpeedKmh, h.WindGustsKmh)
		vStr := fmt.Sprintf("%s %s - %s", rIcon, h.CyclingRating, h.CyclingVerdict)
		if len(vStr) > 28 {
			vStr = vStr[:28]
		}
		modalLines = append(modalLines, fmt.Sprintf("║   %-7s %-18s %-15s %-15s %-18s %s", h.HourLabel, h.WeatherDesc, tStr, pStr, wStr, vStr))
	}

	if m := wSnap.TomorrowMorning5to10; m != nil {
		modalLines = append(modalLines,
			fmt.Sprintf("╠───────────────────────────────────────────────────────────────────────────────────────────────────────╣"),
			fmt.Sprintf("║ 🌅 TOMORROW MORNING (%s 05:00 - 10:00 AM): %s (%d/100) — %s", m.DateStr, m.OverallRating, m.OverallScore, m.Summary),
			fmt.Sprintf("║   Hour    Condition          Temp (Feels)    Precip          Wind / Gusts       Ride Verdict"),
		)
		for _, h := range m.Hours {
			rIcon := "🟢"
			if h.CyclingRating == "FAIR" {
				rIcon = "🟡"
			} else if h.CyclingRating == "POOR" {
				rIcon = "🔴"
			}
			tStr := fmt.Sprintf("%.1f°C (%.1f°C)", h.TempC, h.ApparentTempC)
			pStr := fmt.Sprintf("%d%% (%.1fmm)", h.PrecipProbPct, h.PrecipMM)
			wStr := fmt.Sprintf("%.0f / %.0f km/h", h.WindSpeedKmh, h.WindGustsKmh)
			vStr := fmt.Sprintf("%s %s - %s", rIcon, h.CyclingRating, h.CyclingVerdict)
			if len(vStr) > 28 {
				vStr = vStr[:28]
			}
			modalLines = append(modalLines, fmt.Sprintf("║   %-7s %-18s %-15s %-15s %-18s %s", h.HourLabel, h.WeatherDesc, tStr, pStr, wStr, vStr))
		}
	}

	modalLines = append(modalLines,
		fmt.Sprintf("╠───────────────────────────────────────────────────────────────────────────────────────────────────────╣"),
		fmt.Sprintf("║ Criteria: 🟢 GOOD: 12-28°C, Wind<20km/h, Rain=0mm │ 🟡 FAIR: 9-14°C/29-33°C, Wind 20-30km/h │ 🔴 POOR: Rain≥0.8mm, Wind>32"),
		fmt.Sprintf("║ [W / Esc / Space] Close Window   [R] Refresh Weather Data"),
		fmt.Sprintf("╚═══════════════════════════════════════════════════════════════════════════════════════════════════════╝"),
	)

	buf.WriteString("\n\033[H\n")
	for _, l := range modalLines {
		buf.WriteString(fmt.Sprintf("  %s%s%s\n", th.ModalBg+Bold, l, Reset))
	}
}

func (a *App) drawHelpModalOverlay(buf *strings.Builder) {
	th := a.CurrentTheme()
	helpLines := []string{
		"╔════════════════════════════ POWERTUI KEYBINDINGS & HELP ════════════════════════════╗",
		"║  Q / Ctrl+C  : Quit PowerTUI & print final session energy/cost summary              ║",
		"║  P / Space   : Pause / Resume real-time sensor sampling                             ║",
		"║  R           : Reset session counters & refresh live weather data                   ║",
		"║  + / -       : Increase / decrease refresh rate (0.2s, 0.5s, 1.0s, 2.0s, 5.0s)      ║",
		"║  C           : Cycle color themes (Cyber Neon, Matrix, Sunset, Arctic, Mono)        ║",
		"║  E           : Cycle PSU efficiency profile (80+ Titanium/Plat/Gold/Silver/Bronze)  ║",
		"║  G           : Toggle Power Graph mode (High-res Braille vs Solid Blocks)           ║",
		"║  W           : Toggle Local Weather & Cycling Prediction detail window              ║",
		"║  K           : Edit Electricity Rate (R/kWh) for real-time cost calculation         ║",
		"║  H / ?       : Toggle this help overlay                                             ║",
		"║                                                                                     ║",
		"║  Press [H] or [Esc] to close this help window                                       ║",
		"╚══════════════════════════════════════════════════════════════════════════════════════╝",
	}
	buf.WriteString("\n\033[H\n")
	for _, l := range helpLines {
		buf.WriteString(fmt.Sprintf("    %s%s%s\n", th.ModalBg+Bold, l, Reset))
	}
}

func (a *App) PrintShutdownSummary() {
	now := time.Now()
	durationSec := now.Sub(a.sessionStart).Seconds()
	if durationSec < 0.1 {
		durationSec = 0.1
	}
	durationMin := durationSec / 60.0
	kwh := (a.totalJoules / 3600.0) / 1000.0
	costZar := kwh * a.costPerKwh
	co2Grams := kwh * a.co2PerKwhG
	avgPower := 0.0
	if a.sampleCount > 0 {
		avgPower = a.sumPowerW / float64(a.sampleCount)
	}
	minP := a.minPowerW
	if math.IsInf(minP, 1) {
		minP = 0
	}

	snap := a.currentSnapshot
	wSnap := a.weatherService.GetSnapshot()

	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   ⚡ POWERTUI (GO) - SESSION SHUTDOWN SUMMARY                    ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("• Total Monitored Time : %.1f minutes (%.0f seconds)\n", durationMin, durationSec)
	fmt.Printf("• Total Energy Consumed: %.4f kWh (%.2f Watt-Hours / %.1f kJ)\n", kwh, kwh*1000.0, a.totalJoules/1000.0)
	fmt.Printf("• Total Electricity Cost: R %.4f (ZAR @ R %.2f / kWh)\n", costZar, a.costPerKwh)
	fmt.Printf("• Estimated Carbon Footprint: %.2f g CO₂ (National Grid Avg: 920 g/kWh)\n", co2Grams)
	fmt.Printf("• Power Statistics      : Peak: %.1f W │ Average: %.1f W │ Minimum: %.1f W\n", a.peakPowerW, avgPower, minP)
	fmt.Printf("• PSU Efficiency Profile: %s (%.0f%%)\n", snap.PsuProfile, snap.PsuEfficiency*100.0)
	fmt.Printf("• Hardware Profile      : CPU: %s │ GPU: %s\n", snap.CPU.Model, func() string {
		if len(snap.GPUs) > 0 {
			return snap.GPUs[0].Name
		}
		return "Integrated / None"
	}())

	if wSnap.LastUpdated > 0 {
		fmt.Printf("• Current Location & Weather: %s, %s │ %s %s (%.1f°C) │ 🚲 [%s]\n",
			wSnap.City, wSnap.Country, wSnap.CurrentWeatherIcon, wSnap.CurrentWeatherDesc, wSnap.CurrentTempC, wSnap.CurrentCyclingRating)
	}

	if len(a.sessionTopProcs) > 0 {
		fmt.Println("\nTop Power-Attributed Processes Recorded:")
		var procs []HeavyProcRecord
		for _, p := range a.sessionTopProcs {
			procs = append(procs, p)
		}
		sort.Slice(procs, func(i, j int) bool {
			return procs[i].Power > procs[j].Power
		})
		for i, p := range procs {
			if i >= 5 {
				break
			}
			fmt.Printf("  [%d] %-18s (PID %-6d) : %5.1f W peak │ CPU: %4.1f%% │ User: %s\n", i+1, p.Name, p.Pid, p.Power, p.CPU, p.User)
		}
	}
	fmt.Println("═════════════════════════════════════════════════════════════════════════════════════")
}

func main() {
	app := NewApp()

	// Put terminal in raw mode
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error putting terminal into raw mode: %v\n", err)
		return
	}
	app.oldState = oldState

	// Clean exit handling
	restoreTerminal := func() {
		os.Stdout.WriteString(ShowCursor + AltScreenOff + Reset)
		_ = term.Restore(fd, oldState)
	}
	defer restoreTerminal()

	// Hide cursor and switch to alternate screen buffer
	os.Stdout.WriteString(AltScreenOn + HideCursor + ClearScreen)

	// Listen for SIGINT / SIGTERM / SIGWINCH
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)

	// Channel for user keypresses
	keyChan := make(chan byte, 10)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && err == nil {
				keyChan <- buf[0]
			}
		}
	}()

	// Initial terminal size
	tw, th, _ := term.GetSize(fd)
	app.termWidth = tw
	app.termHeight = th

	ticker := time.NewTicker(app.refreshInterval)
	defer ticker.Stop()

	// Initial data update
	app.UpdateData()
	os.Stdout.WriteString(app.DrawScreen())

	running := true
	for running {
		select {
		case sig := <-sigChan:
			if sig == syscall.SIGWINCH {
				tw, th, _ := term.GetSize(fd)
				app.mu.Lock()
				app.termWidth = tw
				app.termHeight = th
				app.mu.Unlock()
				os.Stdout.WriteString(ClearScreen + app.DrawScreen())
			} else {
				running = false
			}

		case k := <-keyChan:
			switch k {
			case 'q', 'Q', 3: // 'q' or Ctrl+C
				running = false
			case 'p', 'P', ' ':
				if app.showWeatherModal || app.showHelp {
					app.showWeatherModal = false
					app.showHelp = false
				} else {
					app.isPaused = !app.isPaused
				}
				os.Stdout.WriteString(app.DrawScreen())
			case 'r', 'R':
				app.ResetStats()
				app.UpdateData()
				os.Stdout.WriteString(app.DrawScreen())
			case 'w', 'W':
				app.showWeatherModal = !app.showWeatherModal
				app.showHelp = false
				os.Stdout.WriteString(app.DrawScreen())
			case 'h', 'H', '?':
				app.showHelp = !app.showHelp
				app.showWeatherModal = false
				os.Stdout.WriteString(app.DrawScreen())
			case 27: // Escape
				app.showWeatherModal = false
				app.showHelp = false
				os.Stdout.WriteString(app.DrawScreen())
			case 'c', 'C':
				app.themeIdx = (app.themeIdx + 1) % len(Themes)
				os.Stdout.WriteString(app.DrawScreen())
			case 'e', 'E':
				app.psuIdx = (app.psuIdx + 1) % len(app.psuProfiles)
				os.Stdout.WriteString(app.DrawScreen())
			case 'g', 'G':
				if app.graphMode == "braille" {
					app.graphMode = "blocks"
				} else {
					app.graphMode = "braille"
				}
				os.Stdout.WriteString(app.DrawScreen())
			case '+', '=':
				if app.rateIdx > 0 {
					app.rateIdx--
					app.refreshInterval = time.Duration(app.rateOptions[app.rateIdx] * float64(time.Second))
					ticker.Reset(app.refreshInterval)
				}
				os.Stdout.WriteString(app.DrawScreen())
			case '-', '_':
				if app.rateIdx < len(app.rateOptions)-1 {
					app.rateIdx++
					app.refreshInterval = time.Duration(app.rateOptions[app.rateIdx] * float64(time.Second))
					ticker.Reset(app.refreshInterval)
				}
				os.Stdout.WriteString(app.DrawScreen())
			}

		case <-ticker.C:
			if !app.isPaused {
				app.UpdateData()
			}
			os.Stdout.WriteString(app.DrawScreen())
		}
	}

	restoreTerminal()
	app.PrintShutdownSummary()
}
