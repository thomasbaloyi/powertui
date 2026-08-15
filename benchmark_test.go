package main

import (
	"testing"
	"powertui/pkg/sensors"
	"powertui/pkg/weather"
)

func BenchmarkProcessScanningGo(b *testing.B) {
	collector := sensors.NewSensorCollector()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = collector.GetTopProcesses(25.0, 50.0)
	}
}

func BenchmarkSensorSamplingGo(b *testing.B) {
	collector := sensors.NewSensorCollector()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = collector.Sample("80+ Gold")
	}
}

func BenchmarkCyclingEvaluationGo(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = weather.EvaluateCyclingConditions(18.5, 17.8, 10, 0.0, 14.0, 22.0, 1)
		_ = weather.EvaluateCyclingConditions(6.0, 3.2, 85, 4.2, 36.0, 52.0, 63)
		_ = weather.EvaluateCyclingConditions(33.0, 37.5, 5, 0.0, 18.0, 25.0, 0)
	}
}
