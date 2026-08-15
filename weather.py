"""
Weather Forecast & Cycling Prediction Engine for PowerTUI.
Provides automatic IP geolocation, Open-Meteo hourly weather forecasting,
and a meteorological cycling evaluation model predicting ride conditions for:
1. Current real-time conditions
2. Next 3 hours (hour-by-hour)
3. Next day morning ride window (5:00 AM - 10:00 AM)
"""

import json
import threading
import time
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple


# WMO Weather Interpretation Codes (WW)
WMO_CODE_MAP = {
    0: ("Clear sky", "☀️"),
    1: ("Mainly clear", "🌤️"),
    2: ("Partly cloudy", "⛅"),
    3: ("Overcast", "☁️"),
    45: ("Foggy", "🌫️"),
    48: ("Depositing rime fog", "🌫️"),
    51: ("Light drizzle", "🌦️"),
    53: ("Moderate drizzle", "🌦️"),
    55: ("Dense drizzle", "🌧️"),
    56: ("Light freezing drizzle", "🌧️"),
    57: ("Dense freezing drizzle", "🌧️"),
    61: ("Slight rain", "🌧️"),
    63: ("Moderate rain", "🌧️"),
    65: ("Heavy rain", "🌧️"),
    66: ("Light freezing rain", "🌧️"),
    67: ("Heavy freezing rain", "🌧️"),
    71: ("Slight snowfall", "🌨️"),
    73: ("Moderate snowfall", "🌨️"),
    75: ("Heavy snowfall", "🌨️"),
    77: ("Snow grains", "🌨️"),
    80: ("Slight rain showers", "🌦️"),
    81: ("Moderate rain showers", "🌧️"),
    82: ("Violent rain showers", "⛈️"),
    85: ("Slight snow showers", "🌨️"),
    86: ("Heavy snow showers", "🌨️"),
    95: ("Thunderstorm", "⛈️"),
    96: ("Thunderstorm w/ slight hail", "⛈️"),
    99: ("Thunderstorm w/ heavy hail", "⛈️"),
}


def get_weather_desc(code: int) -> Tuple[str, str]:
    return WMO_CODE_MAP.get(code, ("Unknown", "🌡️"))


@dataclass
class CyclingScore:
    rating: str  # "GOOD", "FAIR", "POOR"
    score: int  # 0 to 100
    verdict: str  # Short summary reason
    reasons: List[str] = field(default_factory=list)
    icon: str = "🚲"


def evaluate_cycling_conditions(
    temp_c: float,
    apparent_temp_c: float,
    precip_prob_pct: int,
    precip_mm: float,
    wind_speed_kmh: float,
    wind_gusts_kmh: float,
    weather_code: int
) -> CyclingScore:
    """
    Sound Meteorological Cycling Evaluation Model:
    Evaluates safety, comfort, and performance based on:
    1. Precipitation & Storm hazard (Road grip, braking distance, visibility)
    2. Wind Speed & Gusts (Control stability, resistance, crosswind danger)
    3. Temperature & Apparent Feel (Thermal regulation, freezing risk, heat fatigue)
    4. Fog & Visibility (Traffic collision hazard)
    """
    score = 100
    reasons = []

    # 1. Severe Weather / Storms / Snow / Ice (Strict Dealbreakers)
    if weather_code in (95, 96, 99):
        score = 10
        reasons.append("Thunderstorm / Lightning hazard")
    elif weather_code in (71, 73, 75, 77, 85, 86):
        score = 15
        reasons.append("Snow / Icy roads")
    elif weather_code in (56, 57, 66, 67):
        score = 15
        reasons.append("Freezing rain / Black ice risk")
    elif weather_code in (45, 48):
        score -= 25
        reasons.append("Fog / Reduced road visibility")

    # 2. Rain & Precipitation
    if precip_mm >= 2.0 or precip_prob_pct >= 75:
        score -= 55
        reasons.append(f"Heavy rain expected ({precip_prob_pct}%, {precip_mm:.1f}mm)")
    elif precip_mm >= 0.8 or precip_prob_pct >= 50:
        score -= 40
        reasons.append(f"Rain likely ({precip_prob_pct}%, {precip_mm:.1f}mm)")
    elif precip_mm >= 0.2 or precip_prob_pct >= 25:
        score -= 20
        reasons.append(f"Chance of drizzle ({precip_prob_pct}%, {precip_mm:.1f}mm)")

    # 3. Wind & Gusts
    if wind_gusts_kmh >= 50.0 or wind_speed_kmh >= 35.0:
        score -= 45
        reasons.append(f"Hazardous gusts ({wind_gusts_kmh:.0f} km/h)")
    elif wind_gusts_kmh >= 38.0 or wind_speed_kmh >= 28.0:
        score -= 25
        reasons.append(f"Strong winds ({wind_speed_kmh:.0f} km/h, gusts {wind_gusts_kmh:.0f})")
    elif wind_speed_kmh >= 20.0 or wind_gusts_kmh >= 32.0:
        score -= 12
        reasons.append(f"Breezy ({wind_speed_kmh:.0f} km/h)")

    # 4. Temperature & Thermal Comfort (Apparent Feels-Like)
    if apparent_temp_c < 4.0:
        score -= 45
        reasons.append(f"Near freezing ({apparent_temp_c:.1f}°C feels-like)")
    elif apparent_temp_c < 9.0:
        score -= 20
        reasons.append(f"Chilly ({apparent_temp_c:.1f}°C, wear thermal layers)")
    elif apparent_temp_c < 14.0:
        score -= 6
        reasons.append(f"Crisp/Cool ({apparent_temp_c:.1f}°C)")
    elif apparent_temp_c > 35.0:
        score -= 50
        reasons.append(f"Extreme heat ({apparent_temp_c:.1f}°C, heat stroke risk)")
    elif apparent_temp_c > 30.0:
        score -= 20
        reasons.append(f"Hot ({apparent_temp_c:.1f}°C, heavy hydration needed)")

    score = max(0, min(100, score))

    if score >= 75:
        rating = "GOOD"
        if not reasons:
            verdict = "Great conditions: dry, mild breeze & comfortable temp"
        else:
            verdict = f"Good conditions ({', '.join(reasons)})"
    elif score >= 45:
        rating = "FAIR"
        verdict = f"Fair / Marginal: {', '.join(reasons)}" if reasons else "Fair cycling conditions"
    else:
        rating = "POOR"
        verdict = f"Unfavorable: {', '.join(reasons)}" if reasons else "Poor cycling weather"

    return CyclingScore(rating=rating, score=score, verdict=verdict, reasons=reasons)


@dataclass
class HourlyForecast:
    iso_time: str
    hour_label: str
    temp_c: float
    apparent_temp_c: float
    humidity_pct: int
    precip_prob_pct: int
    precip_mm: float
    wind_speed_kmh: float
    wind_gusts_kmh: float
    weather_code: int
    weather_desc: str
    weather_icon: str
    cycling_rating: str
    cycling_score: int
    cycling_verdict: str


@dataclass
class MorningWindowForecast:
    date_str: str
    overall_rating: str
    overall_score: int
    summary: str
    hours: List[HourlyForecast] = field(default_factory=list)


@dataclass
class WeatherSnapshot:
    city: str = "Detecting Location..."
    country: str = ""
    lat: float = 0.0
    lon: float = 0.0
    timezone: str = "auto"
    last_updated: float = 0.0
    current_temp_c: float = 0.0
    current_apparent_c: float = 0.0
    current_humidity: int = 0
    current_wind_kmh: float = 0.0
    current_wind_gusts_kmh: float = 0.0
    current_precip_mm: float = 0.0
    current_weather_code: int = 0
    current_weather_desc: str = "N/A"
    current_weather_icon: str = "🌡️"
    current_cycling_rating: str = "PENDING"
    current_cycling_verdict: str = "Fetching weather data..."
    next_3_hours: List[HourlyForecast] = field(default_factory=list)
    tomorrow_morning_5_10: Optional[MorningWindowForecast] = None
    is_fetching: bool = False
    error_message: Optional[str] = None


class WeatherService:
    def __init__(self):
        self._lock = threading.Lock()
        self._snapshot = WeatherSnapshot()
        self._last_fetch_time = 0.0
        self._fetch_interval = 900.0  # 15 minutes
        self._thread = None
        self._trigger_fetch()

    def get_snapshot(self) -> WeatherSnapshot:
        with self._lock:
            # Trigger background refresh if stale
            if (time.time() - self._last_fetch_time > self._fetch_interval) and not self._snapshot.is_fetching:
                self._trigger_fetch()
            return self._snapshot

    def refresh_now(self):
        self._trigger_fetch()

    def _trigger_fetch(self):
        if self._thread and self._thread.is_alive():
            return
        self._thread = threading.Thread(target=self._fetch_worker, daemon=True)
        self._thread.start()

    def _fetch_worker(self):
        with self._lock:
            self._snapshot.is_fetching = True

        city = "Cape Town"
        country = "South Africa"
        lat = -33.9258
        lon = 18.4259
        tz = "auto"

        # 1. IP-based Geolocation
        try:
            geo_req = urllib.request.Request(
                "http://ip-api.com/json/?fields=status,message,country,city,lat,lon,timezone",
                headers={"User-Agent": "PowerTUI/1.0"}
            )
            with urllib.request.urlopen(geo_req, timeout=4) as resp:
                geo_data = json.loads(resp.read().decode("utf-8"))
                if geo_data.get("status") == "success":
                    city = geo_data.get("city", city)
                    country = geo_data.get("country", country)
                    lat = geo_data.get("lat", lat)
                    lon = geo_data.get("lon", lon)
                    tz = geo_data.get("timezone", tz)
        except Exception:
            pass  # Fall back to default location

        # 2. Weather & Hourly Forecast from Open-Meteo
        try:
            url = (
                f"https://api.open-meteo.com/v1/forecast?latitude={lat}&longitude={lon}"
                f"&current=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,wind_speed_10m,wind_gusts_10m"
                f"&hourly=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation_probability,precipitation,weather_code,wind_speed_10m,wind_gusts_10m"
                f"&timezone=auto"
            )
            req = urllib.request.Request(url, headers={"User-Agent": "PowerTUI/1.0"})
            with urllib.request.urlopen(req, timeout=6) as resp:
                wdata = json.loads(resp.read().decode("utf-8"))

            current = wdata.get("current", {})
            hourly = wdata.get("hourly", {})

            curr_temp = current.get("temperature_2m", 0.0)
            curr_app = current.get("apparent_temperature", curr_temp)
            curr_hum = current.get("relative_humidity_2m", 0)
            curr_precip = current.get("precipitation", 0.0)
            curr_wcode = current.get("weather_code", 0)
            curr_wind = current.get("wind_speed_10m", 0.0)
            curr_gusts = current.get("wind_gusts_10m", curr_wind)

            w_desc, w_icon = get_weather_desc(curr_wcode)
            curr_cycling = evaluate_cycling_conditions(
                temp_c=curr_temp,
                apparent_temp_c=curr_app,
                precip_prob_pct=0 if curr_precip == 0 else 80,
                precip_mm=curr_precip,
                wind_speed_kmh=curr_wind,
                wind_gusts_kmh=curr_gusts,
                weather_code=curr_wcode
            )

            # Map hourly forecasts
            h_times = hourly.get("time", [])
            time_map = {t: idx for idx, t in enumerate(h_times)}
            now = datetime.now()

            # Next 3 hours (hour-by-hour)
            next_3 = []
            for i in range(1, 4):
                tgt_dt = now + timedelta(hours=i)
                tgt_key = tgt_dt.strftime("%Y-%m-%dT%H:00")
                if tgt_key in time_map:
                    idx = time_map[tgt_key]
                    h_temp = hourly["temperature_2m"][idx]
                    h_app = hourly["apparent_temperature"][idx]
                    h_hum = hourly["relative_humidity_2m"][idx]
                    h_prob = hourly["precipitation_probability"][idx]
                    h_prec = hourly["precipitation"][idx]
                    h_wind = hourly["wind_speed_10m"][idx]
                    h_gust = hourly["wind_gusts_10m"][idx]
                    h_code = hourly["weather_code"][idx]
                    h_desc, h_icon = get_weather_desc(h_code)

                    c_score = evaluate_cycling_conditions(
                        temp_c=h_temp,
                        apparent_temp_c=h_app,
                        precip_prob_pct=h_prob,
                        precip_mm=h_prec,
                        wind_speed_kmh=h_wind,
                        wind_gusts_kmh=h_gust,
                        weather_code=h_code
                    )

                    next_3.append(HourlyForecast(
                        iso_time=tgt_key,
                        hour_label=tgt_dt.strftime("%H:00"),
                        temp_c=h_temp,
                        apparent_temp_c=h_app,
                        humidity_pct=h_hum,
                        precip_prob_pct=h_prob,
                        precip_mm=h_prec,
                        wind_speed_kmh=h_wind,
                        wind_gusts_kmh=h_gust,
                        weather_code=h_code,
                        weather_desc=h_desc,
                        weather_icon=h_icon,
                        cycling_rating=c_score.rating,
                        cycling_score=c_score.score,
                        cycling_verdict=c_score.verdict
                    ))

            # Next day morning window (5:00 AM - 10:00 AM)
            tomorrow = (now + timedelta(days=1)).date()
            morning_hours = []
            for h in range(5, 11):
                tgt_key = f"{tomorrow.strftime('%Y-%m-%d')}T{h:02d}:00"
                if tgt_key in time_map:
                    idx = time_map[tgt_key]
                    h_temp = hourly["temperature_2m"][idx]
                    h_app = hourly["apparent_temperature"][idx]
                    h_hum = hourly["relative_humidity_2m"][idx]
                    h_prob = hourly["precipitation_probability"][idx]
                    h_prec = hourly["precipitation"][idx]
                    h_wind = hourly["wind_speed_10m"][idx]
                    h_gust = hourly["wind_gusts_10m"][idx]
                    h_code = hourly["weather_code"][idx]
                    h_desc, h_icon = get_weather_desc(h_code)

                    c_score = evaluate_cycling_conditions(
                        temp_c=h_temp,
                        apparent_temp_c=h_app,
                        precip_prob_pct=h_prob,
                        precip_mm=h_prec,
                        wind_speed_kmh=h_wind,
                        wind_gusts_kmh=h_gust,
                        weather_code=h_code
                    )

                    morning_hours.append(HourlyForecast(
                        iso_time=tgt_key,
                        hour_label=f"{h:02d}:00",
                        temp_c=h_temp,
                        apparent_temp_c=h_app,
                        humidity_pct=h_hum,
                        precip_prob_pct=h_prob,
                        precip_mm=h_prec,
                        wind_speed_kmh=h_wind,
                        wind_gusts_kmh=h_gust,
                        weather_code=h_code,
                        weather_desc=h_desc,
                        weather_icon=h_icon,
                        cycling_rating=c_score.rating,
                        cycling_score=c_score.score,
                        cycling_verdict=c_score.verdict
                    ))

            morning_forecast = None
            if morning_hours:
                avg_score = int(sum(h.cycling_score for h in morning_hours) / len(morning_hours))
                best_h = max(morning_hours, key=lambda x: x.cycling_score)
                worst_h = min(morning_hours, key=lambda x: x.cycling_score)

                if avg_score >= 75:
                    win_rating = "GOOD"
                    win_summary = f"Great morning ride window! Peak time: {best_h.hour_label} ({best_h.temp_c:.0f}°C, {best_h.wind_speed_kmh:.0f} km/h wind)"
                elif avg_score >= 45:
                    win_rating = "FAIR"
                    win_summary = f"Moderate/Fair morning window. Best slot: {best_h.hour_label} ({best_h.temp_c:.0f}°C, {best_h.cycling_verdict})"
                else:
                    win_rating = "POOR"
                    win_summary = f"Unfavorable morning conditions ({worst_h.cycling_verdict})"

                morning_forecast = MorningWindowForecast(
                    date_str=tomorrow.strftime("%a, %b %d"),
                    overall_rating=win_rating,
                    overall_score=avg_score,
                    summary=win_summary,
                    hours=morning_hours
                )

            with self._lock:
                self._snapshot = WeatherSnapshot(
                    city=city,
                    country=country,
                    lat=lat,
                    lon=lon,
                    timezone=tz,
                    last_updated=time.time(),
                    current_temp_c=curr_temp,
                    current_apparent_c=curr_app,
                    current_humidity=curr_hum,
                    current_wind_kmh=curr_wind,
                    current_wind_gusts_kmh=curr_gusts,
                    current_precip_mm=curr_precip,
                    current_weather_code=curr_wcode,
                    current_weather_desc=w_desc,
                    current_weather_icon=w_icon,
                    current_cycling_rating=curr_cycling.rating,
                    current_cycling_verdict=curr_cycling.verdict,
                    next_3_hours=next_3,
                    tomorrow_morning_5_10=morning_forecast,
                    is_fetching=False,
                    error_message=None
                )
                self._last_fetch_time = time.time()

        except Exception as e:
            with self._lock:
                self._snapshot.is_fetching = False
                self._snapshot.error_message = str(e)
