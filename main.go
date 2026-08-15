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

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
	"powertui/pkg/sensors"
	"powertui/pkg/tui"
	"powertui/pkg/weather"
)

// ANSI Style Constants
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

	// Foregrounds
	FgBlack     = "\033[30m"
	FgRed       = "\033[31m"
	FgGreen     = "\033[32m"
	FgYellow    = "\033[33m"
	FgBlue      = "\033[34m"
	FgMagenta   = "\033[35m"
	FgCyan      = "\033[36m"
	FgWhite     = "\033[37m"

	FgHiBlack   = "\033[90m"
	FgHiRed     = "\033[91m"
	FgHiGreen   = "\033[92m"
	FgHiYellow  = "\033[93m"
	FgHiBlue    = "\033[94m"
	FgHiMagenta = "\033[95m"
	FgHiCyan    = "\033[96m"
	FgHiWhite   = "\033[97m"

	// Backgrounds
	BgBlack     = "\033[40m"
	BgRed       = "\033[41m"
	BgGreen     = "\033[42m"
	BgYellow    = "\033[43m"
	BgBlue      = "\033[44m"
	BgMagenta   = "\033[45m"
	BgCyan      = "\033[46m"
	BgWhite     = "\033[47m"

	BgHiBlack   = "\033[100m"
	BgHiBlue    = "\033[104m"
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
	BoxBorder string
	ModalBg   string
}

var Themes = []Theme{
	{
		Name:      "Cyber Neon",
		Header:    FgHiCyan,
		Accent:    FgHiGreen,
		Warning:   FgHiYellow,
		Danger:    FgHiRed,
		Dim:       FgHiBlack,
		Bar:       FgHiCyan,
		Graph:     FgHiGreen,
		BoxBorder: FgCyan,
		ModalBg:   BgBlack + FgHiCyan,
	},
	{
		Name:      "Matrix Green",
		Header:    FgHiGreen,
		Accent:    FgGreen,
		Warning:   FgHiYellow,
		Danger:    FgHiRed,
		Dim:       FgHiBlack,
		Bar:       FgHiGreen,
		Graph:     FgGreen,
		BoxBorder: FgGreen,
		ModalBg:   BgBlack + FgHiGreen,
	},
	{
		Name:      "Sunset Amber",
		Header:    FgHiYellow,
		Accent:    FgYellow,
		Warning:   FgHiMagenta,
		Danger:    FgHiRed,
		Dim:       FgHiBlack,
		Bar:       FgHiYellow,
		Graph:     FgYellow,
		BoxBorder: FgYellow,
		ModalBg:   BgBlack + FgHiYellow,
	},
	{
		Name:      "Arctic Blue",
		Header:    FgHiBlue,
		Accent:    FgHiCyan,
		Warning:   FgHiYellow,
		Danger:    FgHiRed,
		Dim:       FgHiBlack,
		Bar:       FgHiCyan,
		Graph:     FgHiBlue,
		BoxBorder: FgBlue,
		ModalBg:   BgBlack + FgHiBlue,
	},
	{
		Name:      "Monochrome",
		Header:    FgHiWhite,
		Accent:    FgWhite,
		Warning:   FgHiWhite,
		Danger:    FgHiWhite,
		Dim:       FgHiBlack,
		Bar:       FgWhite,
		Graph:     FgHiWhite,
		BoxBorder: FgHiBlack,
		ModalBg:   BgBlack + FgHiWhite,
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
	screenBuffer     *tui.ScreenBuffer

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
		screenBuffer:    tui.NewScreenBuffer(100, 30),
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

func (a *App) Render() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	w := a.termWidth
	h := a.termHeight
	th := a.CurrentTheme()
	snap := a.currentSnapshot
	sb := a.screenBuffer

	if sb.Width != w || sb.Height != h {
		sb.Resize(w, h)
	}
	sb.Clear()

	if w < 72 || h < 18 {
		sb.DrawString(2, 2, fmt.Sprintf("[ Terminal Too Small: %dx%d (Min 80x24) ]", w, h), Bold+FgHiYellow)
		return sb.Flush()
	}

	// 1. Header Bar (y = 0)
	headerTitle := fmt.Sprintf(" ⚡ POWERTUI │ %s │ %s", snap.CPU.Model, snap.PsuProfile)
	if len(snap.GPUs) > 0 {
		headerTitle += fmt.Sprintf(" │ %s", snap.GPUs[0].Name)
	}
	pauseTag := ""
	if a.isPaused {
		pauseTag = " [PAUSED]"
	}
	rightTag := fmt.Sprintf("Theme: %s │ Refresh: %.1fs%s │ [H]elp [Q]uit ", th.Name, a.refreshInterval.Seconds(), pauseTag)
	sb.FillRect(0, 0, 1, w, ' ', Inverse+th.Header)
	sb.DrawString(0, 0, headerTitle, Inverse+Bold+th.Header)
	if runewidth.StringWidth(rightTag) < w {
		sb.DrawString(0, w-runewidth.StringWidth(rightTag), rightTag, Inverse+th.Header)
	}

	// 2. Power Gauges Lines (y = 1, 2)
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

	// Line 1: Summary Power Draw
	pLine1 := fmt.Sprintf(" ESTIMATED WALL POWER: %6.1f W │ DC SENSORS: %5.1f W │ CPU: %5.1f W │ GPU: %5.1f W │ RAM: %4.1f W │ SSD: %4.1f W │ BOARD: %4.1f W",
		wallW, dcSensorsW, cpuW, gpuW, ramW, diskW, boardW)
	sb.DrawString(1, 0, pLine1, Bold+powerColor)

	// Line 2: PSU Load Progress Bar
	psuBarW := w - 38
	if psuBarW < 12 {
		psuBarW = 12
	}
	barRatio := math.Max(0.0, math.Min(1.0, wallW/math.Max(1.0, a.psuCapacityW)))
	barFilled := int(math.Round(barRatio * float64(psuBarW)))
	if barFilled > psuBarW {
		barFilled = psuBarW
	}
	pBar := strings.Repeat("█", barFilled) + strings.Repeat("░", psuBarW-barFilled)
	psuText := fmt.Sprintf(" PSU Load [%s]: [%s] %5.1f%% (%4.0fW Cap)",
		snap.PsuProfile, pBar, (wallW/a.psuCapacityW)*100.0, a.psuCapacityW)
	sb.DrawString(2, 0, psuText, th.Bar)

	// Layout Grid (y = 3 to h - 2)
	availH := h - 4
	currentY := 3
	isWide := w >= 120

	if isWide {
		leftW := int(float64(w) * 0.55)
		rightW := w - leftW

		graphH := int(math.Max(7.0, math.Min(11.0, float64(availH/2))))
		procH := availH - graphH

		a.drawGraphBox(sb, currentY, 0, graphH, leftW)
		a.drawProcessBox(sb, currentY+graphH, 0, procH, leftW)

		if availH >= 20 {
			subsysH := int(math.Max(6.0, math.Min(9.0, float64(availH-13))))
			weatherH := int(math.Max(6.0, math.Min(8.0, float64(availH-subsysH-4))))
			statsH := availH - subsysH - weatherH

			a.drawSubsystemsBox(sb, currentY, leftW, subsysH, rightW)
			a.drawWeatherBox(sb, currentY+subsysH, leftW, weatherH, rightW)
			a.drawAnalyticsBox(sb, currentY+subsysH+weatherH, leftW, statsH, rightW)
		} else {
			subsysH := int(math.Max(6.0, math.Min(10.0, float64(availH-6))))
			weatherH := availH - subsysH

			a.drawSubsystemsBox(sb, currentY, leftW, subsysH, rightW)
			a.drawWeatherBox(sb, currentY+subsysH, leftW, weatherH, rightW)
		}
	} else {
		// Compact Layout
		graphH := 6
		subsysH := 6
		weatherH := 6
		procH := int(math.Max(5.0, float64(availH-graphH-subsysH-weatherH)))

		a.drawGraphBox(sb, currentY, 0, graphH, w)
		currentY += graphH
		a.drawSubsystemsBox(sb, currentY, 0, subsysH, w)
		currentY += subsysH
		a.drawWeatherBox(sb, currentY, 0, weatherH, w)
		currentY += weatherH
		a.drawProcessBox(sb, currentY, 0, procH, w)
	}

	// 4. Footer Bar (y = h - 1)
	footerY := h - 1
	hotkeys := "[Q]uit  [P]ause  [R]eset  [+/-]Rate  [C]olor  [E]fficiency  [S]ort  [G]raph  [W]eather  [K]Cost  [H]elp"
	sb.FillRect(footerY, 0, 1, w, ' ', Inverse+th.Header)
	sb.DrawString(footerY, 0, hotkeys, Inverse+Bold+th.Header)

	// Modals Overlays
	if a.showWeatherModal {
		a.drawWeatherModal(sb)
	} else if a.showHelp {
		a.drawHelpModal(sb)
	}

	return sb.Flush()
}

func (a *App) drawGraphBox(sb *tui.ScreenBuffer, y, x, h, w int) {
	th := a.CurrentTheme()
	title := fmt.Sprintf("POWER DRAW HISTORY (%s) [Key 'g']", strings.ToUpper(a.graphMode))
	sb.DrawBox(y, x, h, w, title, th.BoxBorder)

	if h < 4 || w < 16 {
		return
	}

	plotH := h - 2
	plotW := w - 12
	if plotW < 10 {
		plotW = 10
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
		graphLines = a.RenderBlocksGraph(a.historyWallW, maxGraphVal, minGraphVal, plotW, plotH)
	} else {
		graphLines = a.RenderBrailleGraph(a.historyWallW, maxGraphVal, minGraphVal, plotW, plotH)
	}

	for idx, gLine := range graphLines {
		yVal := maxGraphVal - (float64(idx)/float64(plotH))*(maxGraphVal-minGraphVal)
		yLabel := fmt.Sprintf("%4.0f W ┤", yVal)
		sb.DrawString(y+1+idx, x+2, yLabel, th.Dim)
		sb.DrawString(y+1+idx, x+2+len(yLabel), gLine, th.Graph)
	}
}

func (a *App) drawProcessBox(sb *tui.ScreenBuffer, y, x, h, w int) {
	th := a.CurrentTheme()
	title := fmt.Sprintf("PROCESS POWER ATTRIBUTION [Sort: %s 's']", strings.ToUpper(a.procSort))
	sb.DrawBox(y, x, h, w, title, th.BoxBorder)

	if h < 3 || w < 30 {
		return
	}

	hdr := "  PID     COMMAND              USER         CPU %   MEM %     EST POWER"
	if w < len(hdr)+4 {
		hdr = "  PID     COMMAND         CPU %   EST POWER"
	}
	sb.DrawString(y+1, x+1, hdr, Bold+th.Header)

	snap := a.currentSnapshot
	curY := y + 2
	maxRows := h - 3

	procs := snap.TopProcesses
	switch a.procSort {
	case "cpu":
		sort.Slice(procs, func(i, j int) bool { return procs[i].CpuPercent > procs[j].CpuPercent })
	case "mem":
		sort.Slice(procs, func(i, j int) bool { return procs[i].MemPercent > procs[j].MemPercent })
	case "pid":
		sort.Slice(procs, func(i, j int) bool { return procs[i].Pid < procs[j].Pid })
	default:
		sort.Slice(procs, func(i, j int) bool { return procs[i].EstimatedWatts > procs[j].EstimatedWatts })
	}

	for i, p := range procs {
		if i >= maxRows {
			break
		}
		pName := p.Name
		if len(pName) > 20 {
			pName = pName[:20]
		}
		pPid := fmt.Sprintf("%d", p.Pid)
		if p.Pid == 0 {
			pPid = "-"
		}

		pColor := Reset
		if p.EstimatedWatts >= 15.0 {
			pColor = Bold + th.Danger
		} else if p.EstimatedWatts >= 5.0 {
			pColor = th.Warning
		}

		row := fmt.Sprintf("  %-7s %-20s %-9s %6.1f%% %6.1f%% %s%10.2f W%s",
			pPid, pName, p.User, p.CpuPercent, p.MemPercent, pColor, p.EstimatedWatts, Reset)
		sb.DrawString(curY, x+1, row, Reset)
		curY++
	}
}

func (a *App) drawSubsystemsBox(sb *tui.ScreenBuffer, y, x, h, w int) {
	th := a.CurrentTheme()
	sb.DrawBox(y, x, h, w, "SUBSYSTEMS & HARDWARE TELEMETRY", th.BoxBorder)

	if h < 3 || w < 20 {
		return
	}

	snap := a.currentSnapshot
	curY := y + 1

	// Line 1: CPU telemetry
	cpuTStr := "N/A"
	if snap.CPU.TemperatureC != nil {
		cpuTStr = fmt.Sprintf("%.0f°C", *snap.CPU.TemperatureC)
	}
	cpuLine := fmt.Sprintf(" • CPU: %5.1f W │ Load: %4.1f%% │ Freq: %4.0f MHz │ Temp: %s",
		snap.CPU.PackagePowerW, snap.CPU.TotalLoad, snap.CPU.AvgFreqMhz, cpuTStr)
	sb.DrawString(curY, x+2, cpuLine, th.Accent)
	curY++

	// Line 2: GPU telemetry
	if len(snap.GPUs) > 0 && curY < y+h-1 {
		g := snap.GPUs[0]
		gTStr := "N/A"
		if g.TemperatureC != nil {
			gTStr = fmt.Sprintf("%.0f°C", *g.TemperatureC)
		}
		gClkStr := "N/A"
		if g.CoreClockMhz != nil {
			gClkStr = fmt.Sprintf("%.0f MHz", *g.CoreClockMhz)
		}
		gpuLine := fmt.Sprintf(" • GPU: %5.1f W │ Clock: %s │ Temp: %s │ VRAM: %.0f MB",
			g.PowerW, gClkStr, gTStr, snap.Bandwidth.GpuVramUsedMb)
		sb.DrawString(curY, x+2, gpuLine, th.Accent)
		curY++
	}

	// Line 3: Memory & Storage
	if curY < y+h-1 {
		ramDisk := fmt.Sprintf(" • RAM: %.1f / %.1f GB (%.1f W) │ Disk I/O: R %.1f MB/s, W %.1f MB/s",
			snap.Other.RamUsedGb, snap.Other.RamTotalGb, snap.Other.RamPowerW, snap.Other.DiskReadMbs, snap.Other.DiskWriteMbs)
		sb.DrawString(curY, x+2, ramDisk, Reset)
		curY++
	}

	// Line 4: Network & Battery
	if curY < y+h-1 {
		netBat := fmt.Sprintf(" • Net: ↓%.2f MB/s, ↑%.2f MB/s │ Battery: %s (%.1f W)",
			snap.Bandwidth.NetRxMbs, snap.Bandwidth.NetTxMbs, snap.Battery.Status, snap.Battery.PowerW)
		sb.DrawString(curY, x+2, netBat, Reset)
		curY++
	}
}

func (a *App) drawWeatherBox(sb *tui.ScreenBuffer, y, x, h, w int) {
	th := a.CurrentTheme()
	sb.DrawBox(y, x, h, w, "WEATHER & CYCLING FORECAST [Key 'w']", th.BoxBorder)

	if h < 3 || w < 20 {
		return
	}

	snap := a.weatherService.GetSnapshot()
	curY := y + 1

	if snap.LastUpdated == 0 && snap.IsFetching {
		sb.DrawString(curY, x+2, "Fetching live weather and cycling forecast...", th.Warning)
		return
	}

	// Line 1: Current Weather & Cycling status
	rColor := FgHiGreen
	rIcon := "🟢"
	if snap.CurrentCyclingRating == "FAIR" {
		rColor = FgHiYellow
		rIcon = "🟡"
	} else if snap.CurrentCyclingRating == "POOR" {
		rColor = FgHiRed
		rIcon = "🔴"
	}

	wLine1 := fmt.Sprintf("📍 %s │ %s %s │ %.1f°C (feels %.1f°C) │ 💨 %.0fkm/h │ 🚲 %s [%s%s%s]",
		snap.City, snap.CurrentWeatherIcon, snap.CurrentWeatherDesc, snap.CurrentTempC, snap.CurrentApparentC, snap.CurrentWindKmh,
		rIcon, Bold+rColor, snap.CurrentCyclingRating, Reset)
	sb.DrawString(curY, x+2, wLine1, Reset)
	curY++

	// Line 2: Next 3 hours (hour-by-hour)
	if len(snap.Next3Hours) > 0 && curY < y+h-1 {
		var hParts []string
		for _, hf := range snap.Next3Hours {
			hIcon := "🟢"
			if hf.CyclingRating == "FAIR" {
				hIcon = "🟡"
			} else if hf.CyclingRating == "POOR" {
				hIcon = "🔴"
			}
			hParts = append(hParts, fmt.Sprintf("%s: %.0f°C 💨%.0fk 💧%d%% %s%s", hf.HourLabel, hf.TempC, hf.WindSpeedKmh, hf.PrecipProbPct, hIcon, hf.CyclingRating))
		}
		sb.DrawString(curY, x+2, "Next 3h: "+strings.Join(hParts, " │ "), th.Accent)
		curY++
	}

	// Line 3: Tomorrow Morning window
	if m := snap.TomorrowMorning5to10; m != nil && curY < y+h-1 {
		mIcon := "🟢"
		mColor := FgHiGreen
		if m.OverallRating == "FAIR" {
			mIcon = "🟡"
			mColor = FgHiYellow
		} else if m.OverallRating == "POOR" {
			mIcon = "🔴"
			mColor = FgHiRed
		}
		mLine := fmt.Sprintf("Tomorrow (05:00-10:00): %s %s%s (%d/100)%s — %s",
			mIcon, Bold+mColor, m.OverallRating, m.OverallScore, Reset, m.Summary)
		sb.DrawString(curY, x+2, mLine, Bold+mColor)
		curY++
	}
}

func (a *App) drawAnalyticsBox(sb *tui.ScreenBuffer, y, x, h, w int) {
	th := a.CurrentTheme()
	sb.DrawBox(y, x, h, w, "CUMULATIVE ENERGY & COST ANALYTICS", th.BoxBorder)

	if h < 3 || w < 20 {
		return
	}

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

	curY := y + 1
	l1 := fmt.Sprintf(" • Energy: %.4f kWh (%.1f kJ) │ Electricity Cost: R %.4f (@ R%.2f/kWh)",
		kwh, a.totalJoules/1000.0, costZar, a.costPerKwh)
	sb.DrawString(curY, x+2, l1, th.Accent)
	curY++

	if curY < y+h-1 {
		l2 := fmt.Sprintf(" • Carbon: %.1f g CO₂ │ Power Draw: Avg %.1f W │ Peak %.1f W │ Min %.1f W",
			co2Grams, avgPower, a.peakPowerW, minP)
		sb.DrawString(curY, x+2, l2, Reset)
		curY++
	}
}

func (a *App) drawWeatherModal(sb *tui.ScreenBuffer) {
	th := a.CurrentTheme()
	wSnap := a.weatherService.GetSnapshot()

	modalW := int(math.Min(108.0, float64(sb.Width-4)))
	modalH := int(math.Min(28.0, float64(sb.Height-2)))
	modalY := int(math.Max(1.0, float64((sb.Height-modalH)/2)))
	modalX := int(math.Max(1.0, float64((sb.Width-modalW)/2)))

	sb.FillRect(modalY, modalX, modalH, modalW, ' ', th.ModalBg)
	sb.DrawDoubleBox(modalY, modalX, modalH, modalW, "LOCAL WEATHER FORECAST & CYCLING OUTLOOK", Bold+th.Header)

	curY := modalY + 2

	// 1. Location & Current Real-Time Conditions
	locHeader := fmt.Sprintf("📍 Location: %s, %s (%.2f°, %.2f°) │ Timezone: %s", wSnap.City, wSnap.Country, wSnap.Lat, wSnap.Lon, wSnap.Timezone)
	sb.DrawString(curY, modalX+2, locHeader, Bold+th.Header)
	curY++

	condText := fmt.Sprintf("Current Weather: %s %s │ Temp: %.1f°C (Feels: %.1f°C) │ Rain: %.1fmm │ Wind: %.0f km/h (Gusts: %.0f) │ Humidity: %d%%",
		wSnap.CurrentWeatherIcon, wSnap.CurrentWeatherDesc, wSnap.CurrentTempC, wSnap.CurrentApparentC, wSnap.CurrentPrecipMM, wSnap.CurrentWindKmh, wSnap.CurrentWindGustsKmh, wSnap.CurrentHumidity)
	sb.DrawString(curY, modalX+2, condText, th.Header)
	curY++

	rIcon := "🟢"
	if wSnap.CurrentCyclingRating == "FAIR" {
		rIcon = "🟡"
	} else if wSnap.CurrentCyclingRating == "POOR" {
		rIcon = "🔴"
	}
	currEval := fmt.Sprintf("Current Ride Verdict: %s [%s] %s", rIcon, wSnap.CurrentCyclingRating, wSnap.CurrentCyclingVerdict)
	sb.DrawString(curY, modalX+2, currEval, Bold+th.Header)
	curY += 2

	// 2. Next 3 Hours Table
	if curY < modalY+modalH-10 {
		sb.DrawString(curY, modalX+2, "── 🕒 NEXT 3 HOURS (HOUR-BY-HOUR CYCLING PREDICTION) ──────────────────────────────────", Bold+th.Header)
		curY++

		hdr := "  Hour    Condition          Temp (Feels)    Precip          Wind / Gusts       Ride Verdict"
		sb.DrawString(curY, modalX+2, hdr, Bold+th.Header)
		curY++

		for _, h := range wSnap.Next3Hours {
			if curY >= modalY+modalH-8 {
				break
			}
			hrIcon := "🟢"
			if h.CyclingRating == "FAIR" {
				hrIcon = "🟡"
			} else if h.CyclingRating == "POOR" {
				hrIcon = "🔴"
			}
			tStr := fmt.Sprintf("%.1f°C (%.1f°C)", h.TempC, h.ApparentTempC)
			pStr := fmt.Sprintf("%d%% (%.1fmm)", h.PrecipProbPct, h.PrecipMM)
			wStr := fmt.Sprintf("%.0f / %.0f km/h", h.WindSpeedKmh, h.WindGustsKmh)
			vStr := fmt.Sprintf("%s %s - %s", hrIcon, h.CyclingRating, h.CyclingVerdict)
			if len(vStr) > 24 {
				vStr = vStr[:24]
			}
			row := fmt.Sprintf("  %-7s %-18s %-15s %-15s %-18s %s", h.HourLabel, h.WeatherDesc, tStr, pStr, wStr, vStr)
			sb.DrawString(curY, modalX+2, row, th.Header)
			curY++
		}
		curY++
	}

	// 3. Tomorrow Morning Window
	if m := wSnap.TomorrowMorning5to10; m != nil && curY < modalY+modalH-6 {
		mIcon := "🟢"
		if m.OverallRating == "FAIR" {
			mIcon = "🟡"
		} else if m.OverallRating == "POOR" {
			mIcon = "🔴"
		}
		secTitle := fmt.Sprintf("── 🌅 TOMORROW MORNING (%s 05:00 - 10:00 AM): %s %s (%d/100) ──────────────────", m.DateStr, mIcon, m.OverallRating, m.OverallScore)
		sb.DrawString(curY, modalX+2, secTitle, Bold+th.Header)
		curY++
		sb.DrawString(curY, modalX+2, "  Summary: "+m.Summary, th.Header)
		curY++

		mHdr := "  Hour    Condition          Temp (Feels)    Precip          Wind / Gusts       Ride Verdict"
		sb.DrawString(curY, modalX+2, mHdr, Bold+th.Header)
		curY++

		for _, h := range m.Hours {
			if curY >= modalY+modalH-4 {
				break
			}
			hrIcon := "🟢"
			if h.CyclingRating == "FAIR" {
				hrIcon = "🟡"
			} else if h.CyclingRating == "POOR" {
				hrIcon = "🔴"
			}
			tStr := fmt.Sprintf("%.1f°C (%.1f°C)", h.TempC, h.ApparentTempC)
			pStr := fmt.Sprintf("%d%% (%.1fmm)", h.PrecipProbPct, h.PrecipMM)
			wStr := fmt.Sprintf("%.0f / %.0f km/h", h.WindSpeedKmh, h.WindGustsKmh)
			vStr := fmt.Sprintf("%s %s - %s", hrIcon, h.CyclingRating, h.CyclingVerdict)
			if len(vStr) > 24 {
				vStr = vStr[:24]
			}
			row := fmt.Sprintf("  %-7s %-18s %-15s %-15s %-18s %s", h.HourLabel, h.WeatherDesc, tStr, pStr, wStr, vStr)
			sb.DrawString(curY, modalX+2, row, th.Header)
			curY++
		}
		curY++
	}

	// 4. Criteria Summary Footer
	crit := "  Criteria: 🟢 GOOD: 12-28°C, Wind<20km/h, Rain=0mm │ 🟡 FAIR: 9-14°C/29-33°C │ 🔴 POOR: Rain≥0.8mm, Wind>32"
	sb.DrawString(modalY+modalH-3, modalX+2, crit, th.Header)

	footerModal := "[W / Esc / Space] Close Window   [R] Refresh Weather Data"
	sb.DrawString(modalY+modalH-2, modalX+3, footerModal, Bold+th.Header)
}

func (a *App) drawHelpModal(sb *tui.ScreenBuffer) {
	th := a.CurrentTheme()
	modalW := int(math.Min(76.0, float64(sb.Width-4)))
	modalH := int(math.Min(22.0, float64(sb.Height-4)))
	modalY := int(math.Max(1.0, float64((sb.Height-modalH)/2)))
	modalX := int(math.Max(1.0, float64((sb.Width-modalW)/2)))

	sb.FillRect(modalY, modalX, modalH, modalW, ' ', th.ModalBg)
	sb.DrawDoubleBox(modalY, modalX, modalH, modalW, "POWERTUI KEYBINDINGS & HELP", Bold+th.Header)

	helpLines := []string{
		"  Q / Ctrl+C  : Quit PowerTUI & print final session energy/cost summary",
		"  P / Space   : Pause / Resume real-time sensor sampling",
		"  R           : Reset session counters & refresh live weather data",
		"  + / -       : Increase / decrease refresh rate (0.2s, 0.5s, 1.0s, 2.0s, 5.0s)",
		"  C           : Cycle color themes (Cyber Neon, Matrix, Sunset, Arctic, Mono)",
		"  E           : Cycle PSU efficiency profile (80+ Titanium/Plat/Gold/Bronze)",
		"  G           : Toggle Power Graph mode (High-res Braille vs Solid Blocks)",
		"  S           : Toggle process sort order (Power, CPU%, Memory%, PID)",
		"  W           : Toggle Local Weather & Cycling Prediction detail window",
		"  K           : Edit Electricity Rate (R/kWh) for real-time cost calculation",
		"  H / ?       : Toggle this help overlay",
		"",
		"  Press [H] or [Esc] to close this help window",
	}

	for i, l := range helpLines {
		if modalY+2+i >= modalY+modalH-1 {
			break
		}
		sb.DrawString(modalY+2+i, modalX+2, l, th.Header)
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

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		app.UpdateData()
		fmt.Print(app.Render())
		app.PrintShutdownSummary()
		return
	}

	// Put terminal in raw mode
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
	if tw <= 0 {
		tw = 100
	}
	if th <= 0 {
		th = 30
	}
	app.termWidth = tw
	app.termHeight = th

	ticker := time.NewTicker(app.refreshInterval)
	defer ticker.Stop()

	// Initial data update
	app.UpdateData()
	os.Stdout.WriteString(app.Render())

	running := true
	for running {
		select {
		case sig := <-sigChan:
			if sig == syscall.SIGWINCH {
				tw, th, _ := term.GetSize(fd)
				if tw > 0 && th > 0 {
					app.mu.Lock()
					app.termWidth = tw
					app.termHeight = th
					app.mu.Unlock()
					os.Stdout.WriteString(ClearScreen + app.Render())
				}
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
				os.Stdout.WriteString(app.Render())
			case 'r', 'R':
				app.ResetStats()
				app.UpdateData()
				os.Stdout.WriteString(app.Render())
			case 'w', 'W':
				app.showWeatherModal = !app.showWeatherModal
				app.showHelp = false
				os.Stdout.WriteString(app.Render())
			case 'h', 'H', '?':
				app.showHelp = !app.showHelp
				app.showWeatherModal = false
				os.Stdout.WriteString(app.Render())
			case 27: // Escape
				app.showWeatherModal = false
				app.showHelp = false
				os.Stdout.WriteString(app.Render())
			case 's', 'S':
				sorts := []string{"power", "cpu", "mem", "pid"}
				curIdx := 0
				for idx, s := range sorts {
					if s == aProcSort(app.procSort) {
						curIdx = idx
						break
					}
				}
				app.procSort = sorts[(curIdx+1)%len(sorts)]
				os.Stdout.WriteString(app.Render())
			case 'c', 'C':
				app.themeIdx = (app.themeIdx + 1) % len(Themes)
				os.Stdout.WriteString(app.Render())
			case 'e', 'E':
				app.psuIdx = (app.psuIdx + 1) % len(app.psuProfiles)
				os.Stdout.WriteString(app.Render())
			case 'g', 'G':
				if app.graphMode == "braille" {
					app.graphMode = "blocks"
				} else {
					app.graphMode = "braille"
				}
				os.Stdout.WriteString(app.Render())
			case '+', '=':
				if app.rateIdx > 0 {
					app.rateIdx--
					app.refreshInterval = time.Duration(app.rateOptions[app.rateIdx] * float64(time.Second))
					ticker.Reset(app.refreshInterval)
				}
				os.Stdout.WriteString(app.Render())
			case '-', '_':
				if app.rateIdx < len(app.rateOptions)-1 {
					app.rateIdx++
					app.refreshInterval = time.Duration(app.rateOptions[app.rateIdx] * float64(time.Second))
					ticker.Reset(app.refreshInterval)
				}
				os.Stdout.WriteString(app.Render())
			}

		case <-ticker.C:
			if !app.isPaused {
				app.UpdateData()
			}
			os.Stdout.WriteString(app.Render())
		}
	}

	restoreTerminal()
	app.PrintShutdownSummary()
}

func aProcSort(s string) string {
	return s
}
