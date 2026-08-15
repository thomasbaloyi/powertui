#!/usr/bin/env python3
"""
Python vs Go Benchmark Comparison Script for PowerTUI.
"""
import time
import os
import resource
from sensors import SensorCollector
from weather import evaluate_cycling_conditions

def benchmark_process_scanning(iterations=100):
    collector = SensorCollector()
    # warm up
    collector.get_top_processes(25.0, 50.0)
    
    start = time.perf_counter()
    for _ in range(iterations):
        _ = collector.get_top_processes(25.0, 50.0)
    dur = time.perf_counter() - start
    avg_ms = (dur / iterations) * 1000.0
    return avg_ms

def benchmark_sensor_sampling(iterations=100):
    collector = SensorCollector()
    collector.sample("80+ Gold")
    
    start = time.perf_counter()
    for _ in range(iterations):
        _ = collector.sample("80+ Gold")
    dur = time.perf_counter() - start
    avg_ms = (dur / iterations) * 1000.0
    return avg_ms

def benchmark_cycling_eval(iterations=100000):
    start = time.perf_counter()
    for _ in range(iterations):
        _ = evaluate_cycling_conditions(18.5, 17.8, 10, 0.0, 14.0, 22.0, 1)
        _ = evaluate_cycling_conditions(6.0, 3.2, 85, 4.2, 36.0, 52.0, 63)
        _ = evaluate_cycling_conditions(33.0, 37.5, 5, 0.0, 18.0, 25.0, 0)
    dur = time.perf_counter() - start
    avg_ns = (dur / iterations) * 1e9
    return avg_ns

def get_rss_kb():
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss

if __name__ == "__main__":
    print("Running Python Benchmarks...")
    proc_ms = benchmark_process_scanning(100)
    sample_ms = benchmark_sensor_sampling(100)
    eval_ns = benchmark_cycling_eval(100000)
    rss_kb = get_rss_kb()
    
    print(f"PYTHON_PROC_SCAN_MS={proc_ms:.4f}")
    print(f"PYTHON_SENSOR_SAMPLE_MS={sample_ms:.4f}")
    print(f"PYTHON_CYCLING_EVAL_NS={eval_ns:.2f}")
    print(f"PYTHON_RSS_KB={rss_kb}")
