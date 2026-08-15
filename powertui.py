#!/usr/bin/env python3
"""
PowerTUI - Modern Real-Time Computer Power Draw Monitor & TUI.
Visualizes CPU, GPU, System Power, Wall Draw, Process Attribution,
and Cumulative Energy / Cost / Carbon analytics in your terminal.
"""

import os
import sys
import time
import curses
import math
import collections
from datetime import datetime, timedelta

from sensors import SensorCollector, PSU_EFFICIENCY_PROFILES, SystemPowerSnapshot
from weather import WeatherService, WeatherSnapshot, HourlyForecast, MorningWindowForecast


# Color Themes
THEMES = {
    "Cyber Neon": {
        "bg": -1,
        "fg_header": curses.COLOR_CYAN,
        "fg_accent": curses.COLOR_GREEN,
        "fg_warning": curses.COLOR_YELLOW,
        "fg_danger": curses.COLOR_RED,
        "fg_dim": curses.COLOR_WHITE,
        "fg_bar": curses.COLOR_CYAN,
        "fg_graph": curses.COLOR_GREEN,
    },
    "Matrix Green": {
        "bg": -1,
        "fg_header": curses.COLOR_GREEN,
        "fg_accent": curses.COLOR_GREEN,
        "fg_warning": curses.COLOR_YELLOW,
        "fg_danger": curses.COLOR_RED,
        "fg_dim": curses.COLOR_WHITE,
        "fg_bar": curses.COLOR_GREEN,
        "fg_graph": curses.COLOR_GREEN,
    },
    "Sunset Amber": {
        "bg": -1,
        "fg_header": curses.COLOR_YELLOW,
        "fg_accent": curses.COLOR_YELLOW,
        "fg_warning": curses.COLOR_MAGENTA,
        "fg_danger": curses.COLOR_RED,
        "fg_dim": curses.COLOR_WHITE,
        "fg_bar": curses.COLOR_YELLOW,
        "fg_graph": curses.COLOR_YELLOW,
    },
    "Arctic Blue": {
        "bg": -1,
        "fg_header": curses.COLOR_BLUE,
        "fg_accent": curses.COLOR_CYAN,
        "fg_warning": curses.COLOR_YELLOW,
        "fg_danger": curses.COLOR_RED,
        "fg_dim": curses.COLOR_WHITE,
        "fg_bar": curses.COLOR_CYAN,
        "fg_graph": curses.COLOR_BLUE,
    },
    "Monochrome": {
        "bg": -1,
        "fg_header": curses.COLOR_WHITE,
        "fg_accent": curses.COLOR_WHITE,
        "fg_warning": curses.COLOR_WHITE,
        "fg_danger": curses.COLOR_WHITE,
        "fg_dim": curses.COLOR_WHITE,
        "fg_bar": curses.COLOR_WHITE,
        "fg_graph": curses.COLOR_WHITE,
    }
}


def format_bytes(b: float) -> str:
    if b < 1024:
        return f"{b:.0f} B"
    elif b < 1024 * 1024:
        return f"{b / 1024:.1f} KB"
    elif b < 1024 * 1024 * 1024:
        return f"{b / (1024 * 1024):.2f} MB"
    elif b < 1024 * 1024 * 1024 * 1024:
        return f"{b / (1024 * 1024 * 1024):.2f} GB"
    else:
        return f"{b / (1024 * 1024 * 1024 * 1024):.3f} TB"


class PowerTUIApp:
    def __init__(self, stdscr):
        self.stdscr = stdscr
        self.collector = SensorCollector()
        
        # State
        self.is_paused = False
        self.refresh_interval = 1.0  # seconds
        self.rate_options = [0.2, 0.5, 1.0, 2.0, 5.0]
        self.rate_idx = 2
        
        self.psu_profiles = list(PSU_EFFICIENCY_PROFILES.keys())
        self.psu_idx = self.psu_profiles.index("80+ Gold") if "80+ Gold" in self.psu_profiles else 0
        self.psu_capacity_w = 750.0  # 750W rated PSU
        
        self.theme_names = list(THEMES.keys())
        self.theme_idx = 0
        
        self.graph_mode = "braille"  # 'braille' or 'blocks'
        self.proc_sort = "power"    # 'power', 'cpu', 'mem', 'pid'
        
        self.cost_per_kwh = 3.50    # South African Rand (ZAR) per kWh (National Eskom/Municipal Avg)
        self.co2_per_kwh_g = 920.0  # grams CO2 per kWh (South African coal-based grid average)
        
        self.show_help = False
        self.show_cost_modal = False
        self.show_weather_modal = False
        self.cost_input_buf = ""
        
        self.weather_service = WeatherService()
        
        # History & Analytics
        self.history_max = 300
        self.history_timestamps = collections.deque(maxlen=self.history_max)
        self.history_wall_w = collections.deque(maxlen=self.history_max)
        self.history_cpu_w = collections.deque(maxlen=self.history_max)
        self.history_gpu_w = collections.deque(maxlen=self.history_max)
        
        self.session_start = time.time()
        self.last_update = time.time()
        self.total_joules = 0.0
        self.peak_power_w = 0.0
        self.min_power_w = float('inf')
        self.sample_count = 0
        self.sum_power_w = 0.0
        self.sum_cpu_w = 0.0
        self.sum_gpu_w = 0.0
        self.session_top_procs = {}
        
        # Gaming peaks
        self.peak_gpu_sclk_mhz = 0.0
        self.peak_gpu_hotspot_c = 0.0
        self.peak_gpu_mem_temp_c = 0.0
        self.peak_cpu_boost_mhz = 0.0
        
        self.current_snapshot: Optional[SystemPowerSnapshot] = None
        self.summary_result = None
        self.init_curses()

    def init_curses(self):
        curses.curs_set(0)
        self.stdscr.nodelay(True)
        self.stdscr.keypad(True)
        curses.start_color()
        curses.use_default_colors()
        self.init_color_pairs()

    def init_color_pairs(self):
        theme = THEMES[self.theme_names[self.theme_idx]]
        # Pair 1: Normal / Dim text
        curses.init_pair(1, theme["fg_dim"], -1)
        # Pair 2: Accent / Cyan / Bright
        curses.init_pair(2, theme["fg_accent"], -1)
        # Pair 3: Header / Title
        curses.init_pair(3, theme["fg_header"], -1)
        # Pair 4: Warning / Yellow
        curses.init_pair(4, theme["fg_warning"], -1)
        # Pair 5: Danger / Red
        curses.init_pair(5, theme["fg_danger"], -1)
        # Pair 6: Bar fill / Graph
        curses.init_pair(6, theme["fg_bar"], -1)
        # Pair 7: Status Inverted / Highlight
        curses.init_pair(7, curses.COLOR_BLACK, theme["fg_header"])
        # Pair 8: Secondary Green
        curses.init_pair(8, curses.COLOR_GREEN, -1)
        # Pair 9: Secondary Magenta/Purple
        curses.init_pair(9, curses.COLOR_MAGENTA, -1)

    def reset_stats(self):
        self.session_start = time.time()
        self.total_joules = 0.0
        self.peak_power_w = 0.0
        self.min_power_w = float('inf')
        self.sample_count = 0
        self.sum_power_w = 0.0
        self.sum_cpu_w = 0.0
        self.sum_gpu_w = 0.0
        self.peak_gpu_sclk_mhz = 0.0
        self.peak_gpu_hotspot_c = 0.0
        self.peak_gpu_mem_temp_c = 0.0
        self.peak_cpu_boost_mhz = 0.0
        self.session_top_procs.clear()
        self.history_timestamps.clear()
        self.history_wall_w.clear()
        self.history_cpu_w.clear()
        self.history_gpu_w.clear()

    def update_data(self):
        profile = self.psu_profiles[self.psu_idx]
        now = time.time()
        dt = now - self.last_update
        self.last_update = now
        
        snapshot = self.collector.sample(psu_profile=profile)
        self.current_snapshot = snapshot
        
        wall_w = snapshot.wall_ac_w
        cpu_w = snapshot.cpu.package_power_w
        gpu_w = sum(g.power_w for g in snapshot.gpus)
        
        self.history_timestamps.append(now)
        self.history_wall_w.append(wall_w)
        self.history_cpu_w.append(cpu_w)
        self.history_gpu_w.append(gpu_w)
        
        # Accumulate Energy
        if dt > 0 and dt < 10.0:
            self.total_joules += wall_w * dt
            
        self.sample_count += 1
        self.sum_power_w += wall_w
        self.sum_cpu_w += cpu_w
        self.sum_gpu_w += gpu_w
        self.peak_power_w = max(self.peak_power_w, wall_w)
        if self.min_power_w == float('inf'):
            self.min_power_w = wall_w
        else:
            self.min_power_w = min(self.min_power_w, wall_w)
            
        # Gaming peaks
        gm = snapshot.gaming
        self.peak_gpu_sclk_mhz = max(self.peak_gpu_sclk_mhz, gm.sclk_mhz)
        if gm.temp_junction_c:
            self.peak_gpu_hotspot_c = max(self.peak_gpu_hotspot_c, gm.temp_junction_c)
        if gm.temp_mem_c:
            self.peak_gpu_mem_temp_c = max(self.peak_gpu_mem_temp_c, gm.temp_mem_c)
        self.peak_cpu_boost_mhz = max(self.peak_cpu_boost_mhz, gm.cpu_peak_boost_mhz)
            
        # Track heavy processes seen during this session
        for p in snapshot.top_processes:
            if p.pid != 0 and p.estimated_watts >= 5.0:
                key = f"{p.name}_{p.pid}"
                if key not in self.session_top_procs or p.estimated_watts > self.session_top_procs[key]["power"]:
                    self.session_top_procs[key] = {
                        "name": p.name,
                        "pid": p.pid,
                        "user": p.user,
                        "power": p.estimated_watts,
                        "cpu": p.cpu_percent,
                        "mem": p.mem_percent
                    }

    def render_braille_graph(self, values: list, max_val: float, min_val: float, width: int, height: int) -> List[str]:
        if width <= 0 or height <= 0:
            return []
        
        # Braille char is 2 columns x 4 rows
        # Grid of dots:
        # col0: [0x1, 0x2, 0x4, 0x40]
        # col1: [0x8, 0x10, 0x20, 0x80]
        dots = [
            [0x01, 0x08],
            [0x02, 0x10],
            [0x04, 0x20],
            [0x40, 0x80]
        ]
        
        total_data_points = width * 2
        total_v_levels = height * 4
        
        # Slice latest data points
        data = list(values)[-total_data_points:]
        if len(data) < total_data_points:
            data = [0.0] * (total_data_points - len(data)) + data
            
        val_range = max(1.0, max_val - min_val)
        
        # Compute discrete level (0 to total_v_levels) for each point
        levels = []
        for v in data:
            norm = max(0.0, min(1.0, (v - min_val) / val_range))
            lvl = int(norm * (total_v_levels - 1))
            levels.append(lvl)
            
        lines = []
        for r in range(height):
            # Row r corresponds to y-levels from (height - 1 - r)*4 + 3 down to (height - 1 - r)*4
            row_idx = height - 1 - r
            chars = []
            for c in range(width):
                l_lvl = levels[c * 2]
                r_lvl = levels[c * 2 + 1]
                
                code = 0x2800
                for dot_r in range(4):
                    dot_lvl = row_idx * 4 + (3 - dot_r)
                    if l_lvl >= dot_lvl:
                        code |= dots[dot_r][0]
                    if r_lvl >= dot_lvl:
                        code |= dots[dot_r][1]
                chars.append(chr(code))
            lines.append("".join(chars))
            
        return lines

    def render_block_graph(self, values: list, max_val: float, min_val: float, width: int, height: int) -> List[str]:
        if width <= 0 or height <= 0:
            return []
        blocks = [" ", " ", "▂", "▃", "▄", "▅", "▆", "▇", "█"]
        data = list(values)[-width:]
        if len(data) < width:
            data = [0.0] * (width - len(data)) + data
            
        val_range = max(1.0, max_val - min_val)
        lines = []
        for r in range(height):
            row_idx = height - 1 - r
            chars = []
            for v in data:
                norm = max(0.0, min(1.0, (v - min_val) / val_range))
                fractional_row = norm * height
                if fractional_row >= row_idx + 1:
                    chars.append("█")
                elif fractional_row <= row_idx:
                    chars.append(" ")
                else:
                    sub = int((fractional_row - row_idx) * 8)
                    sub = max(1, min(8, sub))
                    chars.append(blocks[sub])
            lines.append("".join(chars))
        return lines

    def draw_bar(self, current: float, maximum: float, width: int) -> str:
        if width <= 0:
            return ""
        if maximum <= 0:
            maximum = 1.0
        pct = max(0.0, min(1.0, current / maximum))
        filled_len = int(pct * width)
        empty_len = width - filled_len
        return "█" * filled_len + "░" * empty_len

    def safe_addstr(self, y: int, x: int, text: str, attr=0):
        max_y, max_x = self.stdscr.getmaxyx()
        if y < 0 or y >= max_y or x < 0 or x >= max_x:
            return
        avail = max_x - x - 1
        if avail <= 0:
            return
        try:
            self.stdscr.addstr(y, x, text[:avail], attr)
        except curses.error:
            pass

    def draw_box(self, y: int, x: int, h: int, w: int, title: str = "", attr=0):
        max_y, max_x = self.stdscr.getmaxyx()
        if y + h > max_y or x + w > max_x or h < 2 or w < 2:
            return
        
        # Corners and edges
        tl, tr, bl, br = "┌", "┐", "└", "┘"
        hz, vt = "─", "│"
        
        self.safe_addstr(y, x, tl + hz * (w - 2) + tr, attr)
        for i in range(1, h - 1):
            self.safe_addstr(y + i, x, vt, attr)
            self.safe_addstr(y + i, x + w - 1, vt, attr)
        self.safe_addstr(y + h - 1, x, bl + hz * (w - 2) + br, attr)
        
        if title and w > len(title) + 4:
            self.safe_addstr(y, x + 2, f" {title} ", attr | curses.A_BOLD)

    def draw(self):
        self.stdscr.erase()
        max_y, max_x = self.stdscr.getmaxyx()
        
        if max_y < 20 or max_x < 70:
            msg = f"Terminal too small ({max_x}x{max_y}). Minimum required: 70x20."
            self.safe_addstr(max_y // 2, max(0, (max_x - len(msg)) // 2), msg, curses.color_pair(5) | curses.A_BOLD)
            self.stdscr.refresh()
            return
            
        snap = self.current_snapshot
        if not snap:
            self.safe_addstr(max_y // 2, max(0, (max_x - 14) // 2), "Initializing...", curses.color_pair(2))
            self.stdscr.refresh()
            return

        # 1. Header Bar
        title = " ⚡ POWERTUI v1.0 "
        host_info = f" {os.uname().nodename} | Linux {os.uname().release.split('-')[0]} "
        status = " ❚❚ PAUSED " if self.is_paused else " ● LIVE "
        rate_str = f" Rate: {self.refresh_interval:.1f}s "
        time_str = datetime.now().strftime("%H:%M:%S ")
        
        self.safe_addstr(0, 0, title, curses.color_pair(7) | curses.A_BOLD)
        self.safe_addstr(0, len(title) + 1, host_info, curses.color_pair(1))
        
        st_color = curses.color_pair(4) if self.is_paused else curses.color_pair(8)
        self.safe_addstr(0, max_x - len(status) - len(rate_str) - len(time_str) - 2, status, st_color | curses.A_BOLD)
        self.safe_addstr(0, max_x - len(rate_str) - len(time_str) - 1, rate_str, curses.color_pair(1))
        self.safe_addstr(0, max_x - len(time_str), time_str, curses.color_pair(2) | curses.A_BOLD)
        
        # 2. Main Wall Power Banner (y: 1 to 5)
        banner_h = 5
        banner_w = max_x
        self.draw_box(1, 0, banner_h, banner_w, "SYSTEM POWER DRAW", curses.color_pair(3))
        
        wall_w = snap.wall_ac_w
        meas_w = snap.measured_dc_w
        cpu_w = snap.cpu.package_power_w
        gpu_w = sum(g.power_w for g in snap.gpus)
        other_w = snap.other.ram_power_w + snap.other.disk_power_w + snap.other.motherboard_power_w
        
        # Status rating (scaled for 750W PSU capacity)
        if wall_w < 150:
            load_label = "ECO / LOW"
            load_color = curses.color_pair(8)
        elif wall_w < 350:
            load_label = "MODERATE"
            load_color = curses.color_pair(4)
        elif wall_w < 550:
            load_label = "HIGH LOAD"
            load_color = curses.color_pair(4) | curses.A_BOLD
        else:
            load_label = "PEAK LOAD"
            load_color = curses.color_pair(5) | curses.A_BOLD
            
        p_str = f"⚡ {wall_w:6.1f} W"
        self.safe_addstr(2, 3, p_str, curses.color_pair(2) | curses.A_BOLD)
        self.safe_addstr(2, 3 + len(p_str) + 2, f"[{load_label}]", load_color)
        
        eff_pct = int(snap.psu_efficiency * 100)
        psu_str = f"PSU: {snap.psu_profile} ({eff_pct}% eff) [Key 'e']"
        self.safe_addstr(2, 3 + len(p_str) + len(load_label) + 6, psu_str, curses.color_pair(1))
        
        # Power gauge bar scaled to 750W PSU
        max_scale_w = self.psu_capacity_w
        gauge_width = max(15, min(40, max_x - 45))
        gauge_bar = self.draw_bar(wall_w, max_scale_w, gauge_width)
        gauge_pct = int(min(100.0, (wall_w / max_scale_w) * 100.0))
        gauge_str = f"[{gauge_bar}] {gauge_pct}% of {int(max_scale_w)}W"
        self.safe_addstr(2, max_x - len(gauge_str) - 3, gauge_str, curses.color_pair(6))
        
        # Sub-line breakdown
        breakdown_str = f"Component DC: {snap.total_estimated_dc_w:5.1f}W  │  CPU: {cpu_w:5.1f}W  │  GPU: {gpu_w:5.1f}W  │  RAM: {snap.other.ram_power_w:4.1f}W  │  Drives: {snap.other.disk_power_w:4.1f}W  │  Mobo: {snap.other.motherboard_power_w:4.1f}W"
        self.safe_addstr(3, 3, breakdown_str, curses.color_pair(1))

        # 3. Layout calculation: Top half = Graph + Analytics, Bottom half = Components + Processes
        current_y = 6
        avail_h = max_y - current_y - 2  # leave 2 for footer
        
        # Split vertically or side-by-side depending on terminal width
        is_wide = max_x >= 120
        
        if is_wide:
            left_w = int(max_x * 0.56)
            right_w = max_x - left_w
            
            # Left: Graph (top) + Processes (bottom)
            graph_h = max(7, min(11, avail_h // 2))
            proc_h = avail_h - graph_h
            
            # Right: Subsystems (top) + Weather & Cycling (middle) + Analytics (bottom)
            if avail_h >= 20:
                subsys_h = max(7, min(10, avail_h - 12))
                weather_h = max(6, min(8, avail_h - subsys_h - 4))
                stats_h = avail_h - subsys_h - weather_h
                
                self.draw_graph_box(current_y, 0, graph_h, left_w)
                self.draw_process_box(current_y + graph_h, 0, proc_h, left_w)
                
                self.draw_subsystems_box(current_y, left_w, subsys_h, right_w)
                self.draw_weather_box(current_y + subsys_h, left_w, weather_h, right_w)
                self.draw_analytics_box(current_y + subsys_h + weather_h, left_w, stats_h, right_w)
            else:
                subsys_h = max(7, min(11, avail_h - 6))
                weather_h = avail_h - subsys_h
                
                self.draw_graph_box(current_y, 0, graph_h, left_w)
                self.draw_process_box(current_y + graph_h, 0, proc_h, left_w)
                
                self.draw_subsystems_box(current_y, left_w, subsys_h, right_w)
                self.draw_weather_box(current_y + subsys_h, left_w, weather_h, right_w)
        else:
            # Compact layout: Graph -> Subsystems -> Weather & Cycling -> Processes
            graph_h = 7
            subsys_h = 7
            weather_h = 6
            proc_h = max(5, avail_h - graph_h - subsys_h - weather_h)
            
            self.draw_graph_box(current_y, 0, graph_h, max_x)
            current_y += graph_h
            self.draw_subsystems_box(current_y, 0, subsys_h, max_x)
            current_y += subsys_h
            self.draw_weather_box(current_y, 0, weather_h, max_x)
            current_y += weather_h
            self.draw_process_box(current_y, 0, proc_h, max_x)

        # 4. Footer Bar
        footer_y = max_y - 1
        hotkeys = "[Q]uit  [P]ause  [R]eset  [+/-]Rate  [C]olor  [E]fficiency  [S]ort  [G]raph  [W]eather  [K]Cost  [H]elp"
        self.safe_addstr(footer_y, 0, hotkeys.ljust(max_x - 1), curses.color_pair(7))
        
        # Help / Modals Overlay if active
        if self.show_help:
            self.draw_help_modal()
        elif self.show_cost_modal:
            self.draw_cost_modal()
        elif self.show_weather_modal:
            self.draw_weather_modal()

        self.stdscr.refresh()

    def get_power_color_attr(self, power_w: float):
        # Dynamic color scaled for up to 750W+ systems
        if power_w < 150.0:
            return curses.color_pair(8)  # Green (Eco / Low power <20%)
        elif power_w < 350.0:
            return curses.color_pair(4)  # Yellow / Gold (Moderate power 20-47%)
        elif power_w < 550.0:
            return curses.color_pair(4) | curses.A_BOLD  # Amber / Orange (High power 47-73%)
        else:
            return curses.color_pair(5) | curses.A_BOLD  # Crimson / Red (Peak load >73%)

    def draw_graph_box(self, y: int, x: int, h: int, w: int):
        self.draw_box(y, x, h, w, f"REAL-TIME POWER HISTORY ({self.graph_mode.upper()})", curses.color_pair(3))
        if h < 4 or w < 15:
            return
            
        plot_h = h - 2
        plot_w = w - 8  # reserve 8 chars for Y-axis labels
        
        if not self.history_wall_w:
            return
            
        max_val = max(self.history_wall_w) if self.history_wall_w else 100.0
        min_val = min(self.history_wall_w) if self.history_wall_w else 0.0
        
        # Dynamic autoscale with headroom, cleanly rounded for up to 750W+
        margin = max(10.0, (max_val - min_val) * 0.15)
        graph_max = max(150.0, math.ceil((max_val + margin) / 25.0) * 25.0)
        graph_min = max(0.0, math.floor((min_val - margin) / 25.0) * 25.0)
        
        if self.graph_mode == "braille":
            total_points = plot_w * 2
            raw_data = list(self.history_wall_w)[-total_points:]
            if len(raw_data) < total_points:
                raw_data = [0.0] * (total_points - len(raw_data)) + raw_data
            col_powers = [max(raw_data[2*c], raw_data[2*c + 1]) for c in range(plot_w)]
            lines = self.render_braille_graph(self.history_wall_w, graph_max, graph_min, plot_w, plot_h)
        else:
            raw_data = list(self.history_wall_w)[-plot_w:]
            if len(raw_data) < plot_w:
                raw_data = [0.0] * (plot_w - len(raw_data)) + raw_data
            col_powers = list(raw_data)
            lines = self.render_block_graph(self.history_wall_w, graph_max, graph_min, plot_w, plot_h)
            
        for i, line in enumerate(lines):
            # Y-axis label on top, middle, and bottom
            if i == 0:
                lbl = f"{graph_max:4.0f}W"
            elif i == len(lines) - 1:
                lbl = f"{graph_min:4.0f}W"
            elif i == len(lines) // 2:
                lbl = f"{(graph_max + graph_min)/2:4.0f}W"
            else:
                lbl = "     "
            self.safe_addstr(y + 1 + i, x + 2, lbl, curses.color_pair(1))
            self.safe_addstr(y + 1 + i, x + 7, "│", curses.color_pair(1))
            
            # Render each column in dynamic color matching its instantaneous power level
            for c in range(min(plot_w, len(line))):
                char = line[c]
                col_power = col_powers[c] if c < len(col_powers) else 0.0
                col_attr = self.get_power_color_attr(col_power)
                self.safe_addstr(y + 1 + i, x + 8 + c, char, col_attr)

    def draw_subsystems_box(self, y: int, x: int, h: int, w: int):
        self.draw_box(y, x, h, w, "SUBSYSTEM & GAMING TELEMETRY", curses.color_pair(3))
        snap = self.current_snapshot
        if not snap:
            return
            
        cur_y = y + 1
        # CPU
        cpu = snap.cpu
        c_status = "(RAPL Hardware Sensor)" if cpu.is_rapl_available else "(Estimated - run with sudo for RAPL)"
        cpu_title = f"CPU: {cpu.model[:24]}"
        self.safe_addstr(cur_y, x + 2, cpu_title, curses.color_pair(2) | curses.A_BOLD)
        self.safe_addstr(cur_y, x + len(cpu_title) + 3, f"{cpu.package_power_w:5.1f} W", curses.color_pair(2) | curses.A_BOLD)
        cur_y += 1
        
        bar_len = max(8, min(16, w - 35))
        c_bar = self.draw_bar(cpu.total_load, 100.0, bar_len)
        temp_str = f"{cpu.temperature_c:.0f}°C" if cpu.temperature_c else "N/A"
        freq_str = f"{cpu.avg_freq_mhz:.0f}MHz" if cpu.avg_freq_mhz > 0 else "N/A"
        cpu_sub = f" Load: [{c_bar}] {cpu.total_load:4.1f}% │ Cores: {cpu.core_power_w:4.1f}W │ Temp: {temp_str} │ Freq: {freq_str}"
        self.safe_addstr(cur_y, x + 2, cpu_sub, curses.color_pair(1))
        cur_y += 1
        
        # GPU
        if snap.gpus:
            for g in snap.gpus:
                if cur_y >= y + h - 1:
                    break
                g_title = f"GPU: {g.name[:24]} ({g.vendor})"
                self.safe_addstr(cur_y, x + 2, g_title, curses.color_pair(2) | curses.A_BOLD)
                self.safe_addstr(cur_y, x + len(g_title) + 3, f"{g.power_w:5.1f} W", curses.color_pair(2) | curses.A_BOLD)
                cur_y += 1
                
                g_temp = f"{g.temperature_c:.0f}°C" if g.temperature_c is not None else "N/A"
                g_fan = f"{g.fan_rpm}RPM" if g.fan_rpm is not None else "0 RPM"
                g_clk = f"{g.core_clock_mhz:.0f}MHz" if g.core_clock_mhz is not None else "N/A"
                g_bar = self.draw_bar(g.power_w, g.power_cap_w or 304.0, bar_len)
                gpu_sub = f" Pwr: [{g_bar}] │ Temp: {g_temp} │ Fan: {g_fan} │ Core: {g_clk}"
                self.safe_addstr(cur_y, x + 2, gpu_sub, curses.color_pair(1))
                cur_y += 1
                
        # 🎮 Gaming & Performance HUD
        gm = snap.gaming
        if cur_y < y + h - 1:
            g_tag = "🎮 " if gm.is_gaming else "🖥️ "
            game_color = curses.color_pair(2) | curses.A_BOLD if gm.is_gaming else curses.color_pair(1)
            game_line = f"{g_tag}Game: {gm.active_game[:18]} │ Balance: [{gm.bottleneck_status}] │ Cost: R {gm.gaming_cost_per_hour_zar:.2f}/hr"
            self.safe_addstr(cur_y, x + 2, game_line, game_color)
            cur_y += 1
            
        if cur_y < y + h - 1 and (gm.sclk_mhz > 0 or gm.gpu_busy_pct > 0):
            gpu_3d_bar = self.draw_bar(gm.gpu_busy_pct, 100.0, 8)
            g_perf_line = f"3D Engine: [{gpu_3d_bar}] {gm.gpu_busy_pct:2.0f}% │ SCLK: {gm.sclk_mhz:4.0f}MHz │ MCLK: {gm.mclk_mhz:4.0f}MHz │ Volt: {gm.voltage_mv:4.0f}mV"
            self.safe_addstr(cur_y, x + 2, g_perf_line, curses.color_pair(1))
            cur_y += 1
            
        if cur_y < y + h - 1 and (gm.temp_junction_c is not None or gm.temp_mem_c is not None):
            edge_str = f"{gm.temp_edge_c:.0f}°C" if gm.temp_edge_c else "N/A"
            junc_str = f"{gm.temp_junction_c:.0f}°C" if gm.temp_junction_c else "N/A"
            mem_t_str = f"{gm.temp_mem_c:.0f}°C" if gm.temp_mem_c else "N/A"
            therm_line = f"Thermals: Edge: {edge_str} │ Hotspot: {junc_str} │ GDDR6: {mem_t_str} │ CPU: {temp_str}"
            self.safe_addstr(cur_y, x + 2, therm_line, curses.color_pair(4) if (gm.temp_junction_c and gm.temp_junction_c > 85) else curses.color_pair(1))
            cur_y += 1
                
        # Bandwidth & Data Flow
        bw = snap.bandwidth
        if cur_y < y + h - 1:
            tot_disk_bytes = bw.session_disk_read_bytes + bw.session_disk_write_bytes
            disk_line = f"Storage: R:{bw.disk_read_mbs:5.1f}MB/s W:{bw.disk_write_mbs:5.1f}MB/s │ Session: {format_bytes(tot_disk_bytes):<8} (R:{format_bytes(bw.session_disk_read_bytes)} W:{format_bytes(bw.session_disk_write_bytes)})"
            self.safe_addstr(cur_y, x + 2, disk_line, curses.color_pair(1))
            cur_y += 1
            
        if cur_y < y + h - 1:
            tot_net_bytes = bw.session_net_rx_bytes + bw.session_net_tx_bytes
            net_line = f"Network: Rx:{bw.net_rx_mbs:5.1f}MB/s Tx:{bw.net_tx_mbs:5.1f}MB/s │ Session: {format_bytes(tot_net_bytes):<8} (Rx:{format_bytes(bw.session_net_rx_bytes)} Tx:{format_bytes(bw.session_net_tx_bytes)})"
            self.safe_addstr(cur_y, x + 2, net_line, curses.color_pair(1))
            cur_y += 1
            
        if cur_y < y + h - 1:
            ram_line = f"RAM Bus: {bw.ram_bandwidth_gbs:5.2f}GB/s (Alloc: {bw.ram_alloc_mbs:4.0f}MB/s) │ Processed: {format_bytes(bw.session_ram_bytes)}"
            self.safe_addstr(cur_y, x + 2, ram_line, curses.color_pair(1))
            cur_y += 1
            
        if cur_y < y + h - 1 and bw.gpu_vram_total_mb > 0:
            vram_gb = bw.gpu_vram_used_mb / 1024.0
            vtot_gb = bw.gpu_vram_total_mb / 1024.0
            gpu_line = f"VRAM Bus: {bw.gpu_bandwidth_gbs:5.1f}GB/s ({bw.gpu_mem_busy_pct:2.0f}% busy) │ VRAM: {vram_gb:.1f}/{vtot_gb:.1f}GB (Total: {format_bytes(bw.session_gpu_vram_bytes)})"
            self.safe_addstr(cur_y, x + 2, gpu_line, curses.color_pair(1))
            cur_y += 1
            
        if cur_y < y + h - 1:
            agg_line = f"⚡ Total Bus Flow: {bw.total_throughput_gbs:5.2f} GB/s throughput │ Cumulative Moved: {format_bytes(bw.session_total_data_bytes)}"
            self.safe_addstr(cur_y, x + 2, agg_line, curses.color_pair(2) | curses.A_BOLD)
            cur_y += 1
            
        # Battery if laptop
        if snap.battery.present and cur_y < y + h - 1:
            bat = snap.battery
            rem_str = f"{bat.time_remaining_min // 60}h {bat.time_remaining_min % 60}m" if bat.time_remaining_min else "N/A"
            bat_sub = f"Battery: {bat.status} {bat.capacity_pct}% │ Rate: {bat.power_w:.1f}W │ Est: {rem_str}"
            self.safe_addstr(cur_y, x + 2, bat_sub, curses.color_pair(4))

    def draw_analytics_box(self, y: int, x: int, h: int, w: int):
        self.draw_box(y, x, h, w, "ENERGY, COST & CARBON (SOUTH AFRICA)", curses.color_pair(3))
        if h < 3:
            return
            
        elapsed_sec = max(1.0, time.time() - self.session_start)
        elapsed_td = str(timedelta(seconds=int(elapsed_sec)))
        
        kwh = self.total_joules / (3600.0 * 1000.0)
        wh = self.total_joules / 3600.0
        cost_zar = kwh * self.cost_per_kwh
        co2_g = kwh * self.co2_per_kwh_g
        
        avg_w = (self.sum_power_w / self.sample_count) if self.sample_count > 0 else 0.0
        min_w = self.min_power_w if self.min_power_w != float('inf') else 0.0
        
        line1 = f"Session Time: {elapsed_td} │ Total Energy: {wh:6.2f} Wh ({kwh:6.4f} kWh)"
        line2 = f"Peak Power: {self.peak_power_w:5.1f} W │ Min: {min_w:5.1f} W │ Avg: {avg_w:5.1f} W"
        line3 = f"Est. Cost (@ R {self.cost_per_kwh:.2f}/kWh): R {cost_zar:6.4f} │ Carbon: {co2_g:6.2f} g CO₂"
        
        self.safe_addstr(y + 1, x + 2, line1, curses.color_pair(1))
        if y + 2 < y + h - 1:
            self.safe_addstr(y + 2, x + 2, line2, curses.color_pair(1))
        if y + 3 < y + h - 1:
            self.safe_addstr(y + 3, x + 2, line3, curses.color_pair(2) | curses.A_BOLD)

    def draw_process_box(self, y: int, x: int, h: int, w: int):
        sort_lbl = f"SORT: {self.proc_sort.upper()} [Key 's']"
        self.draw_box(y, x, h, w, f"PROCESS POWER ATTRIBUTION (≥5W FILTERED, {sort_lbl})", curses.color_pair(3))
        if h < 4:
            return
            
        snap = self.current_snapshot
        if not snap or not snap.top_processes:
            self.safe_addstr(y + 1, x + 2, "Scanning process telemetry...", curses.color_pair(1))
            return
            
        # Table Header
        hdr = f"  {'PID':<7} {'NAME':<20} {'USER':<9} {'CPU% (0-100)':>13} {'MEM%':>6} {'EST. POWER':>11}"
        self.safe_addstr(y + 1, x + 1, hdr, curses.color_pair(7) | curses.A_BOLD)
        
        # Separate heavy processes (>=5W) from summary
        heavy_procs = [p for p in snap.top_processes if p.pid != 0 and p.estimated_watts >= 5.0]
        other_proc = next((p for p in snap.top_processes if p.pid == 0), None)
        
        # Sort heavy processes
        if self.proc_sort == "cpu":
            heavy_procs.sort(key=lambda p: p.cpu_percent, reverse=True)
        elif self.proc_sort == "mem":
            heavy_procs.sort(key=lambda p: p.mem_percent, reverse=True)
        elif self.proc_sort == "pid":
            heavy_procs.sort(key=lambda p: p.pid)
        else: # power
            heavy_procs.sort(key=lambda p: p.estimated_watts, reverse=True)
            
        max_rows = h - 3
        cur_row = 0
        
        if not heavy_procs:
            self.safe_addstr(y + 2, x + 3, "No single process consuming ≥ 5.0 W (system idling efficiently)", curses.color_pair(8))
            cur_row += 1
            
        for p in heavy_procs:
            if cur_row >= max_rows - (1 if other_proc else 0):
                break
            row_y = y + 2 + cur_row
            
            p_color = curses.color_pair(1)
            if p.estimated_watts >= 25.0:
                p_color = curses.color_pair(5) | curses.A_BOLD
            elif p.estimated_watts >= 10.0:
                p_color = curses.color_pair(4) | curses.A_BOLD
            elif p.estimated_watts >= 5.0:
                p_color = curses.color_pair(4)
                
            p_line = f"  {p.pid:<7} {p.name[:19]:<20} {p.user[:8]:<9} {p.cpu_percent:>12.1f}% {p.mem_percent:>5.1f}% {p.estimated_watts:>9.2f} W"
            self.safe_addstr(row_y, x + 1, p_line, p_color)
            cur_row += 1
            
        # Draw aggregated summary category for processes < 5W
        if other_proc and cur_row < max_rows:
            row_y = y + 2 + cur_row
            p_line = f"  {'-':<7} {other_proc.name[:19]:<20} {'-':<9} {other_proc.cpu_percent:>12.1f}% {other_proc.mem_percent:>5.1f}% {other_proc.estimated_watts:>9.2f} W"
            self.safe_addstr(row_y, x + 1, p_line, curses.color_pair(1))

    def draw_weather_box(self, y: int, x: int, h: int, w: int):
        self.draw_box(y, x, h, w, "WEATHER & CYCLING FORECAST [Key 'w']", curses.color_pair(3))
        if h < 3 or w < 20:
            return
            
        snap = self.weather_service.get_snapshot()
        if snap.last_updated == 0 and snap.is_fetching:
            self.safe_addstr(y + 1, x + 2, "Fetching live weather and cycling forecast...", curses.color_pair(4))
            return
        if snap.error_message and snap.last_updated == 0:
            self.safe_addstr(y + 1, x + 2, f"Weather unavailable: {snap.error_message[:w-25]}", curses.color_pair(5))
            return
            
        cur_y = y + 1
        # Line 1: Real-time Location & Current Weather + Cycling Rating
        loc_str = f"📍 {snap.city}"
        cond_str = f"{snap.current_weather_icon} {snap.current_weather_desc}"
        temp_str = f"{snap.current_temp_c:.1f}°C (feels {snap.current_apparent_c:.1f}°C)"
        wind_str = f"💨 {snap.current_wind_kmh:.0f}km/h"
        
        rating_color = curses.color_pair(8) if snap.current_cycling_rating == "GOOD" else (curses.color_pair(4) if snap.current_cycling_rating == "FAIR" else curses.color_pair(5))
        rating_str = f"[{snap.current_cycling_rating}]"
        
        l1_prefix = f"{loc_str} │ {cond_str} │ {temp_str} │ {wind_str} │ 🚲 "
        self.safe_addstr(cur_y, x + 2, l1_prefix[:w - 16], curses.color_pair(1))
        self.safe_addstr(cur_y, x + 2 + len(l1_prefix[:w - 16]), rating_str, rating_color | curses.A_BOLD)
        cur_y += 1
        
        # Line 2: Next 3 Hours (Hour-by-hour)
        if snap.next_3_hours and cur_y < y + h - 1:
            h_strs = []
            for hf in snap.next_3_hours[:3]:
                r_icon = "🟢" if hf.cycling_rating == "GOOD" else ("🟡" if hf.cycling_rating == "FAIR" else "🔴")
                h_strs.append(f"{hf.hour_label}: {hf.temp_c:.0f}°C 💨{hf.wind_speed_kmh:.0f}k 💧{hf.precip_prob_pct}% {r_icon}{hf.cycling_rating}")
            line2 = "Next 3h: " + " │ ".join(h_strs)
            self.safe_addstr(cur_y, x + 2, line2[:w - 4], curses.color_pair(2))
            cur_y += 1
            
        # Line 3: Tomorrow Morning Window (5:00 AM - 10:00 AM)
        m = snap.tomorrow_morning_5_10
        if m and cur_y < y + h - 1:
            m_icon = "🟢" if m.overall_rating == "GOOD" else ("🟡" if m.overall_rating == "FAIR" else "🔴")
            m_color = curses.color_pair(8) if m.overall_rating == "GOOD" else (curses.color_pair(4) if m.overall_rating == "FAIR" else curses.color_pair(5))
            m_title = f"Tomorrow (05:00-10:00): {m_icon} {m.overall_rating} ({m.overall_score}/100) — {m.summary}"
            self.safe_addstr(cur_y, x + 2, m_title[:w - 4], m_color | curses.A_BOLD)
            cur_y += 1
            
        # Line 4: Tomorrow 5-10 AM hour-by-hour breakdown
        if m and m.hours and cur_y < y + h - 1:
            m_breakdown = "Morning 5-10am: " + " │ ".join(f"{hr.hour_label[:2]}h:{hr.temp_c:.0f}°C/{'🟢' if hr.cycling_rating == 'GOOD' else ('🟡' if hr.cycling_rating == 'FAIR' else '🔴')}" for hr in m.hours)
            self.safe_addstr(cur_y, x + 2, m_breakdown[:w - 4], curses.color_pair(1))
            cur_y += 1

    def draw_weather_modal(self):
        max_y, max_x = self.stdscr.getmaxyx()
        modal_w = min(108, max_x - 4)
        modal_h = min(28, max_y - 2)
        modal_y = max(1, (max_y - modal_h) // 2)
        modal_x = max(1, (max_x - modal_w) // 2)
        
        # Fill background
        for i in range(modal_h):
            self.safe_addstr(modal_y + i, modal_x, " " * modal_w, curses.color_pair(7))
            
        self.draw_box(modal_y, modal_x, modal_h, modal_w, "LOCAL WEATHER FORECAST & CYCLING OUTLOOK", curses.color_pair(7) | curses.A_BOLD)
        
        snap = self.weather_service.get_snapshot()
        cur_y = modal_y + 2
        
        # 1. Location & Current Real-Time Conditions
        loc_header = f"📍 Location: {snap.city}, {snap.country} ({snap.lat:.2f}°, {snap.lon:.2f}°) │ Timezone: {snap.timezone}"
        self.safe_addstr(cur_y, modal_x + 2, loc_header[:modal_w - 4], curses.color_pair(7) | curses.A_BOLD)
        cur_y += 1
        
        cond_text = f"Current Weather: {snap.current_weather_icon} {snap.current_weather_desc} │ Temp: {snap.current_temp_c:.1f}°C (Feels: {snap.current_apparent_c:.1f}°C) │ Rain: {snap.current_precip_mm:.1f}mm │ Wind: {snap.current_wind_kmh:.0f} km/h (Gusts: {snap.current_wind_gusts_kmh:.0f}) │ Humidity: {snap.current_humidity}%"
        self.safe_addstr(cur_y, modal_x + 2, cond_text[:modal_w - 4], curses.color_pair(7))
        cur_y += 1
        
        r_icon = "🟢" if snap.current_cycling_rating == "GOOD" else ("🟡" if snap.current_cycling_rating == "FAIR" else "🔴")
        curr_eval = f"Current Ride Verdict: {r_icon} [{snap.current_cycling_rating}] {snap.current_cycling_verdict}"
        self.safe_addstr(cur_y, modal_x + 2, curr_eval[:modal_w - 4], curses.color_pair(7) | curses.A_BOLD)
        cur_y += 2
        
        # 2. Next 3 Hours (Hour-by-Hour)
        if cur_y < modal_y + modal_h - 10:
            self.safe_addstr(cur_y, modal_x + 2, "── 🕒 NEXT 3 HOURS (HOUR-BY-HOUR CYCLING PREDICTION) ──".ljust(modal_w - 4, "─"), curses.color_pair(7) | curses.A_BOLD)
            cur_y += 1
            
            hdr = f"  {'Hour':<7} {'Condition':<18} {'Temp (Feels)':<15} {'Precip':<15} {'Wind / Gusts':<18} {'Ride Verdict':<20}"
            self.safe_addstr(cur_y, modal_x + 2, hdr[:modal_w - 4], curses.color_pair(7) | curses.A_BOLD)
            cur_y += 1
            
            for h in snap.next_3_hours[:3]:
                if cur_y >= modal_y + modal_h - 8:
                    break
                hr_icon = "🟢" if h.cycling_rating == "GOOD" else ("🟡" if h.cycling_rating == "FAIR" else "🔴")
                t_str = f"{h.temp_c:.1f}°C ({h.apparent_temp_c:.1f}°C)"
                p_str = f"{h.precip_prob_pct}% ({h.precip_mm:.1f}mm)"
                w_str = f"{h.wind_speed_kmh:.0f} / {h.wind_gusts_kmh:.0f} km/h"
                v_str = f"{hr_icon} {h.cycling_rating} - {h.cycling_verdict[:22]}"
                row = f"  {h.hour_label:<7} {h.weather_desc[:17]:<18} {t_str:<15} {p_str:<15} {w_str:<18} {v_str:<20}"
                self.safe_addstr(cur_y, modal_x + 2, row[:modal_w - 4], curses.color_pair(7))
                cur_y += 1
            cur_y += 1
            
        # 3. Next Day Morning Window (5:00 AM - 10:00 AM)
        m = snap.tomorrow_morning_5_10
        if m and cur_y < modal_y + modal_h - 6:
            m_icon = "🟢" if m.overall_rating == "GOOD" else ("🟡" if m.overall_rating == "FAIR" else "🔴")
            sec_title = f"── 🌅 TOMORROW MORNING ({m.date_str} 05:00 - 10:00 AM): {m_icon} {m.overall_rating} ({m.overall_score}/100) ──"
            self.safe_addstr(cur_y, modal_x + 2, sec_title.ljust(modal_w - 4, "─"), curses.color_pair(7) | curses.A_BOLD)
            cur_y += 1
            self.safe_addstr(cur_y, modal_x + 2, f"  Summary: {m.summary}"[:modal_w - 4], curses.color_pair(7))
            cur_y += 1
            
            m_hdr = f"  {'Hour':<7} {'Condition':<18} {'Temp (Feels)':<15} {'Precip':<15} {'Wind / Gusts':<18} {'Ride Verdict':<20}"
            self.safe_addstr(cur_y, modal_x + 2, m_hdr[:modal_w - 4], curses.color_pair(7) | curses.A_BOLD)
            cur_y += 1
            
            for h in m.hours:
                if cur_y >= modal_y + modal_h - 4:
                    break
                hr_icon = "🟢" if h.cycling_rating == "GOOD" else ("🟡" if h.cycling_rating == "FAIR" else "🔴")
                t_str = f"{h.temp_c:.1f}°C ({h.apparent_temp_c:.1f}°C)"
                p_str = f"{h.precip_prob_pct}% ({h.precip_mm:.1f}mm)"
                w_str = f"{h.wind_speed_kmh:.0f} / {h.wind_gusts_kmh:.0f} km/h"
                v_str = f"{hr_icon} {h.cycling_rating} - {h.cycling_verdict[:22]}"
                row = f"  {h.hour_label:<7} {h.weather_desc[:17]:<18} {t_str:<15} {p_str:<15} {w_str:<18} {v_str:<20}"
                self.safe_addstr(cur_y, modal_x + 2, row[:modal_w - 4], curses.color_pair(7))
                cur_y += 1
            cur_y += 1
            
        # 4. Sound Definition Criteria Summary
        if cur_y < modal_y + modal_h - 2:
            crit = "  Criteria: 🟢 GOOD: 12-28°C, Wind<20km/h, Rain=0mm │ 🟡 FAIR: 9-14°C/29-33°C, Wind 20-30km/h │ 🔴 POOR: Rain≥0.8mm, Wind>32km/h, Storm"
            self.safe_addstr(cur_y, modal_x + 2, crit[:modal_w - 4], curses.color_pair(7))
            
        footer_modal = "[W / Esc / Space] Close Window   [R] Refresh Weather Data"
        self.safe_addstr(modal_y + modal_h - 2, modal_x + 3, footer_modal[:modal_w - 6], curses.color_pair(7) | curses.A_BOLD)

    def draw_help_modal(self):
        max_y, max_x = self.stdscr.getmaxyx()
        modal_w = min(68, max_x - 4)
        modal_h = min(23, max_y - 4)
        modal_y = max(1, (max_y - modal_h) // 2)
        modal_x = max(1, (max_x - modal_w) // 2)
        
        # Fill background
        for i in range(modal_h):
            self.safe_addstr(modal_y + i, modal_x, " " * modal_w, curses.color_pair(7))
            
        self.draw_box(modal_y, modal_x, modal_h, modal_w, "POWERTUI KEYBOARD COMMANDS", curses.color_pair(7) | curses.A_BOLD)
        
        help_lines = [
            ("Q / Ctrl+C", "Quit the application & show summary"),
            ("P / Space", "Pause / Resume live sensor sampling"),
            ("R", "Reset session statistics & cumulative energy"),
            ("+ / -", "Increase / Decrease refresh rate (0.2s - 5s)"),
            ("C", "Cycle color themes (Neon, Matrix, Sunset, Arctic)"),
            ("E", "Cycle PSU Efficiency rating (Titanium, Gold, Bronze...)"),
            ("S", "Sort processes by Power, CPU%, Memory%, or PID"),
            ("G", "Toggle Graph mode between Braille and ASCII Block"),
            ("W", "Toggle Local Weather & Cycling Prediction window"),
            ("K", "Configure electricity rate (R/kWh) for cost stats"),
            ("H / ?", "Toggle this help dialog"),
        ]
        
        cur_y = modal_y + 2
        for key, desc in help_lines:
            if cur_y >= modal_y + modal_h - 2:
                break
            self.safe_addstr(cur_y, modal_x + 3, f"{key:<12}", curses.color_pair(7) | curses.A_BOLD)
            self.safe_addstr(cur_y, modal_x + 16, desc, curses.color_pair(7))
            cur_y += 1
            
        note = "Sensors: Intel/AMD RAPL, AMDGPU sysfs, NVidia-SMI, sysfs hwmon"
        self.safe_addstr(modal_y + modal_h - 2, modal_x + 3, note[:modal_w - 6], curses.color_pair(7))

    def draw_cost_modal(self):
        max_y, max_x = self.stdscr.getmaxyx()
        modal_w = 48
        modal_h = 7
        modal_y = max(1, (max_y - modal_h) // 2)
        modal_x = max(1, (max_x - modal_w) // 2)
        
        for i in range(modal_h):
            self.safe_addstr(modal_y + i, modal_x, " " * modal_w, curses.color_pair(7))
            
        self.draw_box(modal_y, modal_x, modal_h, modal_w, "SET ELECTRICITY COST (ZAR)", curses.color_pair(7) | curses.A_BOLD)
        self.safe_addstr(modal_y + 2, modal_x + 3, f"Current rate: R {self.cost_per_kwh:.2f} / kWh (SA Avg: R3.50)", curses.color_pair(7))
        self.safe_addstr(modal_y + 3, modal_x + 3, f"New rate (R/kWh): {self.cost_input_buf}_", curses.color_pair(7) | curses.A_BOLD)
        self.safe_addstr(modal_y + 5, modal_x + 3, "[Enter] Save   [Esc] Cancel", curses.color_pair(7))

    def handle_input(self):
        try:
            key = self.stdscr.getch()
        except Exception:
            return True
            
        if key == -1:
            return True
            
        # Cost modal input handling
        if self.show_cost_modal:
            if key in [27]:  # Esc
                self.show_cost_modal = False
                self.cost_input_buf = ""
            elif key in [10, 13, curses.KEY_ENTER]:
                try:
                    val = float(self.cost_input_buf)
                    if val >= 0:
                        self.cost_per_kwh = val
                except ValueError:
                    pass
                self.show_cost_modal = False
                self.cost_input_buf = ""
            elif key in [curses.KEY_BACKSPACE, 127, 8]:
                self.cost_input_buf = self.cost_input_buf[:-1]
            elif 48 <= key <= 57 or key == ord('.'):
                if len(self.cost_input_buf) < 8:
                    self.cost_input_buf += chr(key)
            return True

        if self.show_weather_modal:
            if key in [ord('w'), ord('W'), 27, ord('q'), ord('Q'), 10, 13, ord(' ')]:
                self.show_weather_modal = False
            elif key in [ord('r'), ord('R')]:
                self.weather_service.refresh_now()
            return True

        if self.show_help:
            if key in [ord('h'), ord('H'), ord('?'), 27, ord('q'), ord('Q'), 10, 13, ord(' ')]:
                self.show_help = False
            return True

        if key in [ord('q'), ord('Q'), 3]:  # 3 = Ctrl+C
            return False
        elif key in [ord('p'), ord('P'), ord(' ')]:
            self.is_paused = not self.is_paused
        elif key in [ord('r'), ord('R')]:
            self.reset_stats()
            self.weather_service.refresh_now()
        elif key in [ord('+'), ord('='), ord(']')]:
            if self.rate_idx > 0:
                self.rate_idx -= 1
                self.refresh_interval = self.rate_options[self.rate_idx]
        elif key in [ord('-'), ord('_'), ord('[')]:
            if self.rate_idx < len(self.rate_options) - 1:
                self.rate_idx += 1
                self.refresh_interval = self.rate_options[self.rate_idx]
        elif key in [ord('c'), ord('C')]:
            self.theme_idx = (self.theme_idx + 1) % len(self.theme_names)
            self.init_color_pairs()
        elif key in [ord('e'), ord('E')]:
            self.psu_idx = (self.psu_idx + 1) % len(self.psu_profiles)
        elif key in [ord('w'), ord('W')]:
            self.show_weather_modal = True
        elif key in [ord('s'), ord('S')]:
            modes = ["power", "cpu", "mem", "pid"]
            cur_idx = modes.index(self.proc_sort) if self.proc_sort in modes else 0
            self.proc_sort = modes[(cur_idx + 1) % len(modes)]
        elif key in [ord('g'), ord('G')]:
            self.graph_mode = "blocks" if self.graph_mode == "braille" else "braille"
        elif key in [ord('k'), ord('K')]:
            self.show_cost_modal = True
            self.cost_input_buf = f"{self.cost_per_kwh:.2f}"
        elif key in [ord('h'), ord('H'), ord('?')]:
            self.show_help = True
        elif key == curses.KEY_RESIZE:
            curses.update_lines_cols()
            self.stdscr.clear()

        return True

    def get_session_summary(self):
        elapsed_sec = max(1.0, time.time() - self.session_start)
        kwh = self.total_joules / (3600.0 * 1000.0)
        wh = self.total_joules / 3600.0
        cost_zar = kwh * self.cost_per_kwh
        co2_g = kwh * self.co2_per_kwh_g
        avg_w = (self.sum_power_w / self.sample_count) if self.sample_count > 0 else 0.0
        avg_cpu_w = (self.sum_cpu_w / self.sample_count) if self.sample_count > 0 else 0.0
        avg_gpu_w = (self.sum_gpu_w / self.sample_count) if self.sample_count > 0 else 0.0
        min_w = self.min_power_w if self.min_power_w != float('inf') else 0.0
        
        cpu_model = self.collector.cpu_model if hasattr(self.collector, 'cpu_model') else "CPU"
        gpu_name = self.current_snapshot.gpus[0].name if (self.current_snapshot and self.current_snapshot.gpus) else "None"
        
        sorted_procs = sorted(self.session_top_procs.values(), key=lambda x: x["power"], reverse=True)
        bw = self.current_snapshot.bandwidth if (self.current_snapshot and hasattr(self.current_snapshot, 'bandwidth')) else None
        gm = self.current_snapshot.gaming if (self.current_snapshot and hasattr(self.current_snapshot, 'gaming')) else None
        
        return {
            "duration_sec": elapsed_sec,
            "duration_str": str(timedelta(seconds=int(elapsed_sec))),
            "total_joules": self.total_joules,
            "total_wh": wh,
            "total_kwh": kwh,
            "total_cost_zar": cost_zar,
            "cost_rate_zar": self.cost_per_kwh,
            "carbon_g": co2_g,
            "peak_w": self.peak_power_w,
            "min_w": min_w,
            "avg_w": avg_w,
            "cpu_avg_w": avg_cpu_w,
            "gpu_avg_w": avg_gpu_w,
            "psu_profile": self.psu_profiles[self.psu_idx],
            "cpu_model": cpu_model,
            "gpu_name": gpu_name,
            "top_procs": sorted_procs,
            "disk_read_bytes": bw.session_disk_read_bytes if bw else 0,
            "disk_write_bytes": bw.session_disk_write_bytes if bw else 0,
            "net_rx_bytes": bw.session_net_rx_bytes if bw else 0,
            "net_tx_bytes": bw.session_net_tx_bytes if bw else 0,
            "ram_bytes": bw.session_ram_bytes if bw else 0,
            "gpu_vram_bytes": bw.session_gpu_vram_bytes if bw else 0,
            "total_data_bytes": bw.session_total_data_bytes if bw else 0,
            "peak_gpu_sclk_mhz": self.peak_gpu_sclk_mhz,
            "peak_gpu_hotspot_c": self.peak_gpu_hotspot_c,
            "peak_gpu_mem_temp_c": self.peak_gpu_mem_temp_c,
            "peak_cpu_boost_mhz": self.peak_cpu_boost_mhz,
            "active_game": gm.active_game if gm else "Desktop / Idle",
            "gaming_cost_per_hr": gm.gaming_cost_per_hour_zar if gm else 0.0,
            "weather": self.weather_service.get_snapshot(),
        }

    def run(self):
        last_sample = 0.0
        while True:
            now = time.time()
            if not self.is_paused and (now - last_sample >= self.refresh_interval):
                self.update_data()
                last_sample = now
                
            self.draw()
            
            if not self.handle_input():
                break
                
            time.sleep(0.04)  # ~25 FPS input loop


def print_summary(summary_data):
    if not summary_data or summary_data["duration_sec"] < 0.5:
        return
        
    duration = summary_data["duration_str"]
    joules = summary_data["total_joules"]
    wh = summary_data["total_wh"]
    kwh = summary_data["total_kwh"]
    cost = summary_data["total_cost_zar"]
    rate = summary_data["cost_rate_zar"]
    co2 = summary_data["carbon_g"]
    peak_w = summary_data["peak_w"]
    min_w = summary_data["min_w"]
    avg_w = summary_data["avg_w"]
    psu = summary_data["psu_profile"]
    cpu_model = summary_data["cpu_model"]
    cpu_avg_w = summary_data["cpu_avg_w"]
    gpu_name = summary_data["gpu_name"]
    gpu_avg_w = summary_data["gpu_avg_w"]
    heavy_procs = summary_data["top_procs"]
    w_snap = summary_data.get("weather")
    
    BOLD = "\033[1m"
    GREEN = "\033[32m"
    CYAN = "\033[36m"
    YELLOW = "\033[33m"
    DIM = "\033[2m"
    RESET = "\033[0m"
    
    print("\n" + BOLD + CYAN + "╔══════════════════════════════════════════════════════════════════════════════╗" + RESET)
    print(BOLD + CYAN + "║                        ⚡ PowerTUI Session Summary                            ║" + RESET)
    print(BOLD + CYAN + "╠══════════════════════════════════════════════════════════════════════════════╣" + RESET)
    print(f"║ {BOLD}Session Duration       :{RESET} {duration:<53} ║")
    print(f"║ {BOLD}Total Energy Consumed  :{RESET} {GREEN}{wh:6.2f} Wh{RESET} ({kwh:6.4f} kWh / {int(joules):,} J){'':<20} ║")
    print(f"║ {BOLD}Total Electricity Cost :{RESET} {YELLOW}R {cost:7.4f}{RESET} (at R {rate:.2f}/kWh [South African Rand]){'':<9} ║")
    print(f"║ {BOLD}Carbon Footprint       :{RESET} {co2:6.2f} g CO₂ (at 920 g/kWh SA coal grid average){'':<8} ║")
    print(BOLD + CYAN + "╟──────────────────────────────────────────────────────────────────────────────╢" + RESET)
    print(f"║ {BOLD}POWER CONSUMPTION METRICS (AC WALL DRAW){'':<40}║")
    print(f"║   • Peak Power Draw    : {BOLD}{peak_w:6.1f} W{RESET}{'':<49} ║")
    print(f"║   • Minimum Power Draw : {BOLD}{min_w:6.1f} W{RESET}{'':<49} ║")
    print(f"║   • Average Power Draw : {BOLD}{avg_w:6.1f} W{RESET}{'':<49} ║")
    print(f"║   • PSU Efficiency     : {psu:<52} ║")
    print(BOLD + CYAN + "╟──────────────────────────────────────────────────────────────────────────────╢" + RESET)
    print(f"║ {BOLD}SUBSYSTEM BREAKDOWN (AVERAGE ESTIMATES){'':<41}║")
    print(f"║   • CPU ({cpu_model[:22]}): ~{cpu_avg_w:5.1f} W (RAPL Package){'':<19} ║")
    if gpu_name != "None":
        print(f"║   • GPU ({gpu_name[:22]}): ~{gpu_avg_w:5.1f} W (Hardware Sensor){'':<18} ║")
    platform_avg = max(0.0, avg_w * 0.90 - cpu_avg_w - gpu_avg_w)
    print(f"║   • Platform & RAM Baseline : ~{platform_avg:5.1f} W (RAM, NVMe, Motherboard & Fans){'':<7} ║")
    
    print(BOLD + CYAN + "╟──────────────────────────────────────────────────────────────────────────────╢" + RESET)
    print(f"║ {BOLD}🎮 GAMING TELEMETRY & THERMAL PEAKS{'':<45}║")
    active_g = summary_data.get('active_game', 'Desktop / Idle')
    cost_hr = summary_data.get('gaming_cost_per_hr', 0.0)
    print(f"║   • Active 3D Application : {active_g[:28]:<28} (R {cost_hr:.2f}/hr gaming){'':<7} ║")
    sclk_p = summary_data.get('peak_gpu_sclk_mhz', 0.0)
    cpu_b = summary_data.get('peak_cpu_boost_mhz', 0.0)
    print(f"║   • Peak GPU Shader Clock : {sclk_p:6.0f} MHz  │ Peak CPU Boost: {cpu_b:6.0f} MHz{'':<13} ║")
    junc_p = summary_data.get('peak_gpu_hotspot_c', 0.0)
    mem_p = summary_data.get('peak_gpu_mem_temp_c', 0.0)
    print(f"║   • Peak GPU Junction/Hotspot: {junc_p:4.0f}°C   │ Peak GDDR6 Temp: {mem_p:4.0f}°C{'':<16} ║")
    
    print(BOLD + CYAN + "╟──────────────────────────────────────────────────────────────────────────────╢" + RESET)
    print(f"║ {BOLD}DATA & BANDWIDTH FLOW TOTALS (SESSION LIFETIME){'':<33}║")
    d_r = format_bytes(summary_data.get('disk_read_bytes', 0))
    d_w = format_bytes(summary_data.get('disk_write_bytes', 0))
    d_tot = format_bytes(summary_data.get('disk_read_bytes', 0) + summary_data.get('disk_write_bytes', 0))
    print(f"║   • Storage I/O (Drives)  : {d_r:>8} read  │ {d_w:>8} written ({d_tot:<8} total){'':<10} ║")
    
    n_rx = format_bytes(summary_data.get('net_rx_bytes', 0))
    n_tx = format_bytes(summary_data.get('net_tx_bytes', 0))
    n_tot = format_bytes(summary_data.get('net_rx_bytes', 0) + summary_data.get('net_tx_bytes', 0))
    print(f"║   • Network Traffic (LAN) : {n_rx:>8} rx    │ {n_tx:>8} tx      ({n_tot:<8} total){'':<10} ║")
    
    ram_tot = format_bytes(summary_data.get('ram_bytes', 0))
    print(f"║   • RAM Memory Data Flow  : {ram_tot:>8} processed (allocations & cache){'':<15} ║")
    
    if summary_data.get('gpu_vram_bytes', 0) > 0:
        vram_tot = format_bytes(summary_data.get('gpu_vram_bytes', 0))
        print(f"║   • GPU VRAM Memory Flow  : {vram_tot:>8} processed (GDDR6 bus activity){'':<18} ║")
        
    tot_data = format_bytes(summary_data.get('total_data_bytes', 0))
    print(f"║   • Total System Data Moved: {BOLD}{tot_data:>8}{RESET} aggregate flow{'':<33} ║")
    
    # 🌤️ Local Weather & Cycling Prediction
    if w_snap and w_snap.last_updated > 0:
        print(BOLD + CYAN + "╟──────────────────────────────────────────────────────────────────────────────╢" + RESET)
        print(f"║ {BOLD}🌤️ LOCAL WEATHER & CYCLING FORECAST INTELLIGENCE{'':<32}║")
        w_loc = f"• Location: {w_snap.city}, {w_snap.country} │ {w_snap.current_weather_icon} {w_snap.current_weather_desc} │ {w_snap.current_temp_c:.1f}°C (Feels {w_snap.current_apparent_c:.1f}°C)"
        print(f"║   {w_loc[:74]:<74} ║")
        w_curr = f"• Current Cycling Verdict: [{w_snap.current_cycling_rating}] {w_snap.current_cycling_verdict}"
        print(f"║   {w_curr[:74]:<74} ║")
        
        if w_snap.next_3_hours:
            print(f"║   • Next 3 Hours (Hour-by-Hour Cycling Prediction):{'':<30} ║")
            for h in w_snap.next_3_hours[:3]:
                r_tag = "GOOD" if h.cycling_rating == "GOOD" else ("FAIR" if h.cycling_rating == "FAIR" else "POOR")
                h_line = f"    - {h.hour_label}: {h.temp_c:.0f}°C, Wind {h.wind_speed_kmh:.0f}km/h (G:{h.wind_gusts_kmh:.0f}), Rain {h.precip_prob_pct}% -> [{r_tag}] {h.cycling_verdict}"
                print(f"║   {h_line[:74]:<74} ║")
                
        m = w_snap.tomorrow_morning_5_10
        if m:
            m_line = f"• Tomorrow Morning (05:00-10:00): [{m.overall_rating}] ({m.overall_score}/100) — {m.summary}"
            print(f"║   {m_line[:74]:<74} ║")
            if m.hours:
                m_row = "    " + " │ ".join(f"{hr.hour_label[:2]}h:{hr.temp_c:.0f}°C/[{hr.cycling_rating[:1]}]" for hr in m.hours)
                print(f"║   {m_row[:74]:<74} ║")
    
    if heavy_procs:
        print(BOLD + CYAN + "╟──────────────────────────────────────────────────────────────────────────────╢" + RESET)
        print(f"║ {BOLD}TOP HIGH-POWER PROCESSES OBSERVED (≥ 5W){'':<40}║")
        for p in heavy_procs[:5]:
            p_line = f"• {p['name']} (PID {p['pid']}, {p['user']}): peak {p['power']:.1f}W ({p['cpu']:.1f}% CPU)"
            print(f"║   {p_line:<72} ║")
            
    print(BOLD + CYAN + "╚══════════════════════════════════════════════════════════════════════════════╝" + RESET)
    print(DIM + " PowerTUI session ended cleanly.\n" + RESET)


def main():
    summary = None
    try:
        def runner(stdscr):
            app = PowerTUIApp(stdscr)
            app.run()
            return app.get_session_summary()
        summary = curses.wrapper(runner)
    except KeyboardInterrupt:
        pass
    except Exception as e:
        print(f"Error during execution: {e}")
        
    print_summary(summary)

if __name__ == "__main__":
    main()
