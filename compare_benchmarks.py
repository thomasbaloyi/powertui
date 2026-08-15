#!/usr/bin/env python3
import subprocess
import time
import json
import os
import resource

def run_benchmarks():
    print("Building Go binary...")
    subprocess.run(["go", "build", "-o", "powertui-bin", "main.go"], check=True)
    
    # 1. Measure Binary Sizes & RSS Memory
    go_bin_size_kb = os.path.getsize("powertui-bin") / 1024.0
    
    print("Running Python Benchmarks...")
    py_proc = subprocess.run(["python3", "benchmark_py.py"], capture_output=True, text=True, check=True)
    py_lines = py_proc.stdout.strip().split("\n")
    py_metrics = {}
    for line in py_lines:
        if "=" in line:
            k, v = line.split("=", 1)
            py_metrics[k] = float(v)
            
    print("Running Go Benchmarks...")
    go_proc = subprocess.run(["go", "test", "-bench=.", "-benchmem", "-benchtime=2s"], capture_output=True, text=True, check=True)
    go_lines = go_proc.stdout.strip().split("\n")
    go_metrics = {}
    for line in go_lines:
        if line.startswith("Benchmark"):
            parts = line.split()
            name = parts[0].split("-")[0]
            ns_op = float(parts[2])
            bytes_op = float(parts[4])
            allocs_op = float(parts[6])
            go_metrics[name] = {
                "ns_op": ns_op,
                "ms_op": ns_op / 1e6,
                "bytes_op": bytes_op,
                "allocs_op": allocs_op
            }
            
    return py_metrics, go_metrics, go_bin_size_kb

if __name__ == "__main__":
    py_m, go_m, bin_kb = run_benchmarks()
    
    print("\n" + "="*70)
    print("                   BENCHMARK RESULTS: PYTHON vs GO")
    print("="*70)
    
    py_proc_ms = py_m.get("PYTHON_PROC_SCAN_MS", 0)
    go_proc_ms = go_m.get("BenchmarkProcessScanningGo", {}).get("ms_op", 0)
    proc_speedup = (py_proc_ms / go_proc_ms) if go_proc_ms > 0 else 0
    
    py_sample_ms = py_m.get("PYTHON_SENSOR_SAMPLE_MS", 0)
    go_sample_ms = go_m.get("BenchmarkSensorSamplingGo", {}).get("ms_op", 0)
    sample_speedup = (py_sample_ms / go_sample_ms) if go_sample_ms > 0 else 0
    
    py_eval_ns = py_m.get("PYTHON_CYCLING_EVAL_NS", 0)
    go_eval_ns = go_m.get("BenchmarkCyclingEvaluationGo", {}).get("ns_op", 0)
    eval_speedup = (py_eval_ns / go_eval_ns) if go_eval_ns > 0 else 0
    
    print(f"1. Process Scraping (/proc/*) Latency:")
    print(f"   • Python : {py_proc_ms:.3f} ms / sample")
    print(f"   • Go     : {go_proc_ms:.3f} ms / sample  ({proc_speedup:.2f}x faster)")
    
    print(f"\n2. Full Sensor Sampling Loop (CPU, GPU, RAM, Disks, Procs, Thermals):")
    print(f"   • Python : {py_sample_ms:.3f} ms / sample")
    print(f"   • Go     : {go_sample_ms:.3f} ms / sample  ({sample_speedup:.2f}x faster)")
    
    print(f"\n3. Cycling Weather Evaluator (3 queries):")
    print(f"   • Python : {py_eval_ns:.1f} ns")
    print(f"   • Go     : {go_eval_ns:.1f} ns  ({eval_speedup:.2f}x faster)")
    
    print(f"\n4. Memory Footprint (RSS / Binary):")
    print(f"   • Python Runtime RSS : ~{py_m.get('PYTHON_RSS_KB', 0) / 1024.0:.1f} MB")
    print(f"   • Go Standalone Exec : {bin_kb / 1024.0:.2f} MB (Single static executable, zero pip dependencies)")
    print("="*70 + "\n")
