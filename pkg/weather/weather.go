package weather

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// WMO Weather Interpretation Codes (WW)
var WMOCodeMap = map[int][2]string{
	0:  {"Clear sky", "☀️"},
	1:  {"Mainly clear", "🌤️"},
	2:  {"Partly cloudy", "⛅"},
	3:  {"Overcast", "☁️"},
	45: {"Foggy", "🌫️"},
	48: {"Depositing rime fog", "🌫️"},
	51: {"Light drizzle", "🌦️"},
	53: {"Moderate drizzle", "🌦️"},
	55: {"Dense drizzle", "🌧️"},
	56: {"Light freezing drizzle", "🌧️"},
	57: {"Dense freezing drizzle", "🌧️"},
	61: {"Slight rain", "🌧️"},
	63: {"Moderate rain", "🌧️"},
	65: {"Heavy rain", "🌧️"},
	66: {"Light freezing rain", "🌧️"},
	67: {"Heavy freezing rain", "🌧️"},
	71: {"Slight snowfall", "🌨️"},
	73: {"Moderate snowfall", "🌨️"},
	75: {"Heavy snowfall", "🌨️"},
	77: {"Snow grains", "🌨️"},
	80: {"Slight rain showers", "🌦️"},
	81: {"Moderate rain showers", "🌧️"},
	82: {"Violent rain showers", "⛈️"},
	85: {"Slight snow showers", "🌨️"},
	86: {"Heavy snow showers", "🌨️"},
	95: {"Thunderstorm", "⛈️"},
	96: {"Thunderstorm w/ slight hail", "⛈️"},
	99: {"Thunderstorm w/ heavy hail", "⛈️"},
}

func GetWeatherDesc(code int) (string, string) {
	if val, ok := WMOCodeMap[code]; ok {
		return val[0], val[1]
	}
	return "Unknown", "🌡️"
}

type CyclingScore struct {
	Rating  string   `json:"rating"` // "GOOD", "FAIR", "POOR"
	Score   int      `json:"score"`  // 0 to 100
	Verdict string   `json:"verdict"`
	Reasons []string `json:"reasons"`
	Icon    string   `json:"icon"`
}

func EvaluateCyclingConditions(
	tempC float64,
	apparentTempC float64,
	precipProbPct int,
	precipMM float64,
	windSpeedKmh float64,
	windGustsKmh float64,
	weatherCode int,
) CyclingScore {
	score := 100
	var reasons []string

	// 1. Severe Weather / Storms / Snow / Ice (Strict Dealbreakers)
	switch weatherCode {
	case 95, 96, 99:
		score = 10
		reasons = append(reasons, "Thunderstorm / Lightning hazard")
	case 71, 73, 75, 77, 85, 86:
		score = 15
		reasons = append(reasons, "Snow / Icy roads")
	case 56, 57, 66, 67:
		score = 15
		reasons = append(reasons, "Freezing rain / Black ice risk")
	case 45, 48:
		score -= 25
		reasons = append(reasons, "Fog / Reduced road visibility")
	}

	// 2. Rain & Precipitation
	if precipMM >= 2.0 || precipProbPct >= 75 {
		score -= 55
		reasons = append(reasons, fmt.Sprintf("Heavy rain expected (%d%%, %.1fmm)", precipProbPct, precipMM))
	} else if precipMM >= 0.8 || precipProbPct >= 50 {
		score -= 40
		reasons = append(reasons, fmt.Sprintf("Rain likely (%d%%, %.1fmm)", precipProbPct, precipMM))
	} else if precipMM >= 0.2 || precipProbPct >= 25 {
		score -= 20
		reasons = append(reasons, fmt.Sprintf("Chance of drizzle (%d%%, %.1fmm)", precipProbPct, precipMM))
	}

	// 3. Wind & Gusts
	if windGustsKmh >= 50.0 || windSpeedKmh >= 35.0 {
		score -= 45
		reasons = append(reasons, fmt.Sprintf("Hazardous gusts (%.0f km/h)", windGustsKmh))
	} else if windGustsKmh >= 38.0 || windSpeedKmh >= 28.0 {
		score -= 25
		reasons = append(reasons, fmt.Sprintf("Strong winds (%.0f km/h, gusts %.0f)", windSpeedKmh, windGustsKmh))
	} else if windSpeedKmh >= 20.0 || windGustsKmh >= 32.0 {
		score -= 12
		reasons = append(reasons, fmt.Sprintf("Breezy (%.0f km/h)", windSpeedKmh))
	}

	// 4. Temperature & Thermal Comfort (Apparent Feels-Like)
	if apparentTempC < 4.0 {
		score -= 45
		reasons = append(reasons, fmt.Sprintf("Near freezing (%.1f°C feels-like)", apparentTempC))
	} else if apparentTempC < 9.0 {
		score -= 20
		reasons = append(reasons, fmt.Sprintf("Chilly (%.1f°C, wear thermal layers)", apparentTempC))
	} else if apparentTempC < 14.0 {
		score -= 6
		reasons = append(reasons, fmt.Sprintf("Crisp/Cool (%.1f°C)", apparentTempC))
	} else if apparentTempC > 35.0 {
		score -= 50
		reasons = append(reasons, fmt.Sprintf("Extreme heat (%.1f°C, heat stroke risk)", apparentTempC))
	} else if apparentTempC > 30.0 {
		score -= 20
		reasons = append(reasons, fmt.Sprintf("Hot (%.1f°C, heavy hydration needed)", apparentTempC))
	}

	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	var rating, verdict string
	if score >= 75 {
		rating = "GOOD"
		if len(reasons) == 0 {
			verdict = "Great conditions: dry, mild breeze & comfortable temp"
		} else {
			verdict = "Good conditions (" + joinStrings(reasons, ", ") + ")"
		}
	} else if score >= 45 {
		rating = "FAIR"
		if len(reasons) > 0 {
			verdict = "Fair / Marginal: " + joinStrings(reasons, ", ")
		} else {
			verdict = "Fair cycling conditions"
		}
	} else {
		rating = "POOR"
		if len(reasons) > 0 {
			verdict = "Unfavorable: " + joinStrings(reasons, ", ")
		} else {
			verdict = "Poor cycling weather"
		}
	}

	return CyclingScore{
		Rating:  rating,
		Score:   score,
		Verdict: verdict,
		Reasons: reasons,
		Icon:    "🚲",
	}
}

func joinStrings(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	res := elems[0]
	for _, e := range elems[1:] {
		res += sep + e
	}
	return res
}

type HourlyForecast struct {
	ISOTime        string  `json:"iso_time"`
	HourLabel      string  `json:"hour_label"`
	TempC          float64 `json:"temp_c"`
	ApparentTempC  float64 `json:"apparent_temp_c"`
	HumidityPct    int     `json:"humidity_pct"`
	PrecipProbPct  int     `json:"precip_prob_pct"`
	PrecipMM       float64 `json:"precip_mm"`
	WindSpeedKmh   float64 `json:"wind_speed_kmh"`
	WindGustsKmh   float64 `json:"wind_gusts_kmh"`
	WeatherCode    int     `json:"weather_code"`
	WeatherDesc    string  `json:"weather_desc"`
	WeatherIcon    string  `json:"weather_icon"`
	CyclingRating  string  `json:"cycling_rating"`
	CyclingScore   int     `json:"cycling_score"`
	CyclingVerdict string  `json:"cycling_verdict"`
}

type MorningWindowForecast struct {
	DateStr       string           `json:"date_str"`
	OverallRating string           `json:"overall_rating"`
	OverallScore  int              `json:"overall_score"`
	Summary       string           `json:"summary"`
	Hours         []HourlyForecast `json:"hours"`
}

type WeatherSnapshot struct {
	City                  string                 `json:"city"`
	Country               string                 `json:"country"`
	Lat                   float64                `json:"lat"`
	Lon                   float64                `json:"lon"`
	Timezone              string                 `json:"timezone"`
	LastUpdated           float64                `json:"last_updated"`
	CurrentTempC          float64                `json:"current_temp_c"`
	CurrentApparentC      float64                `json:"current_apparent_c"`
	CurrentHumidity       int                    `json:"current_humidity"`
	CurrentWindKmh        float64                `json:"current_wind_kmh"`
	CurrentWindGustsKmh   float64                `json:"current_wind_gusts_kmh"`
	CurrentPrecipMM       float64                `json:"current_precip_mm"`
	CurrentWeatherCode    int                    `json:"current_weather_code"`
	CurrentWeatherDesc    string                 `json:"current_weather_desc"`
	CurrentWeatherIcon    string                 `json:"current_weather_icon"`
	CurrentCyclingRating  string                 `json:"current_cycling_rating"`
	CurrentCyclingVerdict string                 `json:"current_cycling_verdict"`
	Next3Hours            []HourlyForecast       `json:"next_3_hours"`
	TomorrowMorning5to10  *MorningWindowForecast `json:"tomorrow_morning_5_10"`
	IsFetching            bool                   `json:"is_fetching"`
	ErrorMessage          string                 `json:"error_message"`
}

type WeatherService struct {
	mu            sync.RWMutex
	snapshot      WeatherSnapshot
	lastFetchTime float64
	fetchInterval float64
	client        *http.Client
}

func NewWeatherService() *WeatherService {
	ws := &WeatherService{
		snapshot: WeatherSnapshot{
			City:                  "Detecting Location...",
			CurrentWeatherDesc:    "N/A",
			CurrentWeatherIcon:    "🌡️",
			CurrentCyclingRating:  "PENDING",
			CurrentCyclingVerdict: "Fetching weather data...",
		},
		fetchInterval: 900.0, // 15 mins
		client: &http.Client{
			Timeout: 6 * time.Second,
		},
	}
	ws.TriggerFetch()
	return ws
}

func (ws *WeatherService) GetSnapshot() WeatherSnapshot {
	ws.mu.Lock()
	if time.Now().Unix()-int64(ws.lastFetchTime) > int64(ws.fetchInterval) && !ws.snapshot.IsFetching {
		go ws.fetchWorker()
	}
	ws.mu.Unlock()

	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.snapshot
}

func (ws *WeatherService) RefreshNow() {
	ws.TriggerFetch()
}

func (ws *WeatherService) TriggerFetch() {
	ws.mu.Lock()
	if ws.snapshot.IsFetching {
		ws.mu.Unlock()
		return
	}
	ws.snapshot.IsFetching = true
	ws.mu.Unlock()

	go ws.fetchWorker()
}

func (ws *WeatherService) fetchWorker() {
	city := "Cape Town"
	country := "South Africa"
	lat := -33.9258
	lon := 18.4259
	tz := "auto"

	// 1. IP Geolocation
	reqGeo, err := http.NewRequest("GET", "http://ip-api.com/json/?fields=status,message,country,city,lat,lon,timezone", nil)
	if err == nil {
		reqGeo.Header.Set("User-Agent", "PowerTUI-Go/1.0")
		respGeo, err := ws.client.Do(reqGeo)
		if err == nil && respGeo.StatusCode == 200 {
			var geoData struct {
				Status   string  `json:"status"`
				Country  string  `json:"country"`
				City     string  `json:"city"`
				Lat      float64 `json:"lat"`
				Lon      float64 `json:"lon"`
				Timezone string  `json:"timezone"`
			}
			if err := json.NewDecoder(respGeo.Body).Decode(&geoData); err == nil && geoData.Status == "success" {
				city = geoData.City
				country = geoData.Country
				lat = geoData.Lat
				lon = geoData.Lon
				tz = geoData.Timezone
			}
			respGeo.Body.Close()
		}
	}

	// 2. Open-Meteo Forecast
	url := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&current=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,wind_speed_10m,wind_gusts_10m"+
			"&hourly=temperature_2m,relative_humidity_2m,apparent_temperature,precipitation_probability,precipitation,weather_code,wind_speed_10m,wind_gusts_10m"+
			"&timezone=auto",
		lat, lon,
	)

	reqW, err := http.NewRequest("GET", url, nil)
	if err != nil {
		ws.mu.Lock()
		ws.snapshot.IsFetching = false
		ws.snapshot.ErrorMessage = err.Error()
		ws.mu.Unlock()
		return
	}
	reqW.Header.Set("User-Agent", "PowerTUI-Go/1.0")

	respW, err := ws.client.Do(reqW)
	if err != nil {
		ws.mu.Lock()
		ws.snapshot.IsFetching = false
		ws.snapshot.ErrorMessage = err.Error()
		ws.mu.Unlock()
		return
	}
	defer respW.Body.Close()

	var wdata struct {
		Current struct {
			Temperature2m       float64 `json:"temperature_2m"`
			ApparentTemperature float64 `json:"apparent_temperature"`
			RelativeHumidity2m  int     `json:"relative_humidity_2m"`
			Precipitation       float64 `json:"precipitation"`
			WeatherCode         int     `json:"weather_code"`
			WindSpeed10m        float64 `json:"wind_speed_10m"`
			WindGusts10m        float64 `json:"wind_gusts_10m"`
		} `json:"current"`
		Hourly struct {
			Time                     []string  `json:"time"`
			Temperature2m            []float64 `json:"temperature_2m"`
			ApparentTemperature      []float64 `json:"apparent_temperature"`
			RelativeHumidity2m       []int     `json:"relative_humidity_2m"`
			PrecipitationProbability []int     `json:"precipitation_probability"`
			Precipitation            []float64 `json:"precipitation"`
			WeatherCode              []int     `json:"weather_code"`
			WindSpeed10m             []float64 `json:"wind_speed_10m"`
			WindGusts10m             []float64 `json:"wind_gusts_10m"`
		} `json:"hourly"`
	}

	if err := json.NewDecoder(respW.Body).Decode(&wdata); err != nil {
		ws.mu.Lock()
		ws.snapshot.IsFetching = false
		ws.snapshot.ErrorMessage = err.Error()
		ws.mu.Unlock()
		return
	}

	curr := wdata.Current
	wDesc, wIcon := GetWeatherDesc(curr.WeatherCode)
	precipProb := 0
	if curr.Precipitation > 0 {
		precipProb = 80
	}
	currCycling := EvaluateCyclingConditions(
		curr.Temperature2m,
		curr.ApparentTemperature,
		precipProb,
		curr.Precipitation,
		curr.WindSpeed10m,
		curr.WindGusts10m,
		curr.WeatherCode,
	)

	// Build map for hourly lookups
	timeMap := make(map[string]int)
	for idx, t := range wdata.Hourly.Time {
		timeMap[t] = idx
	}

	now := time.Now()
	// Next 3 hours
	var next3 []HourlyForecast
	for i := 1; i <= 3; i++ {
		tgtDt := now.Add(time.Duration(i) * time.Hour)
		tgtKey := tgtDt.Format("2006-01-02T15:00")
		if idx, ok := timeMap[tgtKey]; ok && idx < len(wdata.Hourly.Temperature2m) {
			hTemp := wdata.Hourly.Temperature2m[idx]
			hApp := wdata.Hourly.ApparentTemperature[idx]
			hHum := wdata.Hourly.RelativeHumidity2m[idx]
			hProb := wdata.Hourly.PrecipitationProbability[idx]
			hPrec := wdata.Hourly.Precipitation[idx]
			hWind := wdata.Hourly.WindSpeed10m[idx]
			hGust := wdata.Hourly.WindGusts10m[idx]
			hCode := wdata.Hourly.WeatherCode[idx]
			hDesc, hIcon := GetWeatherDesc(hCode)

			cScore := EvaluateCyclingConditions(hTemp, hApp, hProb, hPrec, hWind, hGust, hCode)
			next3 = append(next3, HourlyForecast{
				ISOTime:        tgtKey,
				HourLabel:      tgtDt.Format("15:00"),
				TempC:          hTemp,
				ApparentTempC:  hApp,
				HumidityPct:    hHum,
				PrecipProbPct:  hProb,
				PrecipMM:       hPrec,
				WindSpeedKmh:   hWind,
				WindGustsKmh:   hGust,
				WeatherCode:    hCode,
				WeatherDesc:    hDesc,
				WeatherIcon:    hIcon,
				CyclingRating:  cScore.Rating,
				CyclingScore:   cScore.Score,
				CyclingVerdict: cScore.Verdict,
			})
		}
	}

	// Tomorrow morning (05:00 - 10:00)
	tomorrow := now.Add(24 * time.Hour)
	var morningHours []HourlyForecast
	for h := 5; h <= 10; h++ {
		tgtKey := fmt.Sprintf("%sT%02d:00", tomorrow.Format("2006-01-02"), h)
		if idx, ok := timeMap[tgtKey]; ok && idx < len(wdata.Hourly.Temperature2m) {
			hTemp := wdata.Hourly.Temperature2m[idx]
			hApp := wdata.Hourly.ApparentTemperature[idx]
			hHum := wdata.Hourly.RelativeHumidity2m[idx]
			hProb := wdata.Hourly.PrecipitationProbability[idx]
			hPrec := wdata.Hourly.Precipitation[idx]
			hWind := wdata.Hourly.WindSpeed10m[idx]
			hGust := wdata.Hourly.WindGusts10m[idx]
			hCode := wdata.Hourly.WeatherCode[idx]
			hDesc, hIcon := GetWeatherDesc(hCode)

			cScore := EvaluateCyclingConditions(hTemp, hApp, hProb, hPrec, hWind, hGust, hCode)
			morningHours = append(morningHours, HourlyForecast{
				ISOTime:        tgtKey,
				HourLabel:      fmt.Sprintf("%02d:00", h),
				TempC:          hTemp,
				ApparentTempC:  hApp,
				HumidityPct:    hHum,
				PrecipProbPct:  hProb,
				PrecipMM:       hPrec,
				WindSpeedKmh:   hWind,
				WindGustsKmh:   hGust,
				WeatherCode:    hCode,
				WeatherDesc:    hDesc,
				WeatherIcon:    hIcon,
				CyclingRating:  cScore.Rating,
				CyclingScore:   cScore.Score,
				CyclingVerdict: cScore.Verdict,
			})
		}
	}

	var morningForecast *MorningWindowForecast
	if len(morningHours) > 0 {
		sumScore := 0
		bestH := morningHours[0]
		worstH := morningHours[0]
		for _, mh := range morningHours {
			sumScore += mh.CyclingScore
			if mh.CyclingScore > bestH.CyclingScore {
				bestH = mh
			}
			if mh.CyclingScore < worstH.CyclingScore {
				worstH = mh
			}
		}
		avgScore := int(math.Round(float64(sumScore) / float64(len(morningHours))))
		var winRating, winSummary string
		if avgScore >= 75 {
			winRating = "GOOD"
			winSummary = fmt.Sprintf("Great morning ride window! Peak time: %s (%.0f°C, %.0f km/h wind)", bestH.HourLabel, bestH.TempC, bestH.WindSpeedKmh)
		} else if avgScore >= 45 {
			winRating = "FAIR"
			winSummary = fmt.Sprintf("Moderate/Fair morning window. Best slot: %s (%.0f°C, %s)", bestH.HourLabel, bestH.TempC, bestH.CyclingVerdict)
		} else {
			winRating = "POOR"
			winSummary = fmt.Sprintf("Unfavorable morning conditions (%s)", worstH.CyclingVerdict)
		}

		morningForecast = &MorningWindowForecast{
			DateStr:       tomorrow.Format("Mon, Jan 02"),
			OverallRating: winRating,
			OverallScore:  avgScore,
			Summary:       winSummary,
			Hours:         morningHours,
		}
	}

	ws.mu.Lock()
	ws.snapshot = WeatherSnapshot{
		City:                  city,
		Country:               country,
		Lat:                   lat,
		Lon:                   lon,
		Timezone:              tz,
		LastUpdated:           float64(time.Now().Unix()),
		CurrentTempC:          curr.Temperature2m,
		CurrentApparentC:      curr.ApparentTemperature,
		CurrentHumidity:       curr.RelativeHumidity2m,
		CurrentWindKmh:        curr.WindSpeed10m,
		CurrentWindGustsKmh:   curr.WindGusts10m,
		CurrentPrecipMM:       curr.Precipitation,
		CurrentWeatherCode:    curr.WeatherCode,
		CurrentWeatherDesc:    wDesc,
		CurrentWeatherIcon:    wIcon,
		CurrentCyclingRating:  currCycling.Rating,
		CurrentCyclingVerdict: currCycling.Verdict,
		Next3Hours:            next3,
		TomorrowMorning5to10:  morningForecast,
		IsFetching:            false,
		ErrorMessage:          "",
	}
	ws.lastFetchTime = float64(time.Now().Unix())
	ws.mu.Unlock()
}
