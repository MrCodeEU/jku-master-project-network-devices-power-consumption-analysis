#!/usr/bin/env python3
"""
Preprocess Test 4 (Wi-Fi Distance & Band) CSVs into clean pgfplots-compatible files.
Run from report/ directory:  python3 preprocess_test4.py

Test 4: Wi-Fi power & throughput at two distances (near / wall) and two bands (2.4 / 5 GHz).

Data sources:
  - Asus, Fritzbox, Alcatel: "auto" CSVs with inline iperf3 throughput
  - Huawei: "manual" CSVs (power only) + separate iperf3 .txt files (1-sec intervals)
  - Alcatel: Extra "both_wifi" tests where the PC is also on Wi-Fi
  - Asus: We prefer auto CSVs (they have inline throughput)

Produces:
  data/test4_summary.csv          - One row per test scenario (for bar charts)
  data/test4_<device>_<scenario>_data.csv - Time-series for line plots
"""
import csv
import statistics
import os
import re
import sys


# ─── Test definitions ──────────────────────────────────────────────────────
# Each entry: (device, band, distance, csv_path, speed_txt_path_or_None, label)
BASE = "../testing/saved_tests/test4"

TESTS = [
    # Asus — auto CSVs (Load Enabled: true, inline throughput)
    ("asus",     "5",   "near", f"{BASE}/asus5.csv",          None, "Asus 5 GHz Near"),
    ("asus",     "5",   "wall", f"{BASE}/asus5_wall.csv",     None, "Asus 5 GHz Wall"),
    ("asus",     "2.4", "near", f"{BASE}/asus2.4.csv",        None, "Asus 2.4 GHz Near"),
    ("asus",     "2.4", "wall", f"{BASE}/asus2.4_wall.csv",   None, "Asus 2.4 GHz Wall"),

    # Fritzbox — auto CSVs (both radios always on, cannot disable individual band)
    ("fritzbox", "5",   "near", f"{BASE}/fritzbox5.csv",          None, "Fritzbox 5 GHz Near"),
    ("fritzbox", "5",   "wall", f"{BASE}/fritzbox5_wall.csv",     None, "Fritzbox 5 GHz Wall"),
    ("fritzbox", "2.4", "near", f"{BASE}/fritzbox2.4.csv",        None, "Fritzbox 2.4 GHz Near"),
    ("fritzbox", "2.4", "wall", f"{BASE}/fritzbox2.4_wall.csv",   None, "Fritzbox 2.4 GHz Wall"),

    # Huawei — manual CSVs + separate iperf3 speed .txt files
    ("huawei",   "5",   "near", f"{BASE}/huawei5.csv",     f"{BASE}/huawei5_speed.txt",      "Huawei 5 GHz Near"),
    ("huawei",   "5",   "wall", f"{BASE}/huawei5_wall.csv", f"{BASE}/huawei5_wall_speed.txt", "Huawei 5 GHz Wall"),
    ("huawei",   "2.4", "near", f"{BASE}/huawei2.4.csv",   f"{BASE}/huawei2.4_speed.txt",    "Huawei 2.4 GHz Near"),
    ("huawei",   "2.4", "wall", f"{BASE}/huawei2.4_wall.csv", f"{BASE}/huawei2.4_wall_speed.txt", "Huawei 2.4 GHz Wall"),

    # Alcatel — 2.4 GHz only, auto CSVs
    ("alcatel",  "2.4", "near", f"{BASE}/alcatel2.4.csv",       None, "Alcatel 2.4 GHz Near"),
    ("alcatel",  "2.4", "wall", f"{BASE}/alcatel2.4_wall.csv",  None, "Alcatel 2.4 GHz Wall"),

    # Alcatel — extra test: PC also on Wi-Fi (to bypass 100 Mbps LAN bottleneck)
    ("alcatel",  "2.4", "near_wifi", f"{BASE}/alcaltel2.4_both_wifi.csv",      None, "Alcatel 2.4 GHz Near (WiFi PC)"),
    ("alcatel",  "2.4", "wall_wifi", f"{BASE}/alcatel2.4_both_wifi_wall.csv",  None, "Alcatel 2.4 GHz Wall (WiFi PC)"),
]


# ─── Parsing helpers ───────────────────────────────────────────────────────

def parse_csv(path):
    """Parse CSV skipping comment lines starting with #.
    Handles double-quoted whole-row entries (e.g. manuall/asus_5_wall.csv)."""
    rows = []
    header = None
    with open(path, "r", encoding="utf-8-sig") as f:
        for line in f:
            line = line.strip()
            if line.startswith("#") or not line:
                continue

            # Handle double-quoted rows: entire row wrapped in an outer quote
            # e.g. "2026-02-22T11:23:13...,0,5860,0,0,pre,""[phase]..."""
            if header is not None and line.startswith('"') and line.count('"') > 4:
                # Strip outer quotes and unescape inner doubled quotes
                inner = line[1:-1] if line.endswith('"') else line[1:]
                inner = inner.replace('""', '"')
                line = inner

            if header is None:
                header = [h.strip() for h in line.split(",")]
                continue
            reader = csv.reader([line])
            for parsed in reader:
                rows.append(parsed)
    return header, rows


def find_col(header, name):
    """Case-insensitive column lookup."""
    name_l = name.lower()
    for i, h in enumerate(header):
        if h.lower().strip() == name_l:
            return i
    return None


def parse_iperf3_txt(path):
    """Parse iperf3 text output and return list of (second, bitrate_mbps).

    Handles both 'Mbits/sec' and 'bits/sec' (for 0 entries).
    Returns empty list if file is empty or missing.
    """
    if not os.path.exists(path):
        return []
    entries = []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            # Match per-second lines like:
            # [  5]   0.00-1.00   sec  2.75 MBytes  23.1 Mbits/sec    2    156 KBytes
            # [  5]  69.00-70.00  sec  0.00 Bytes  0.00 bits/sec    0   77.8 KBytes
            m = re.match(
                r'\[\s*\d+\]\s+'
                r'([\d.]+)-([\d.]+)\s+sec\s+'
                r'[\d.]+\s+[A-Za-z]+\s+'
                r'([\d.]+)\s+'
                r'([A-Za-z/]+)',
                line
            )
            if m:
                start_sec = float(m.group(1))
                end_sec = float(m.group(2))
                bitrate = float(m.group(3))
                unit = m.group(4).lower()

                # Convert to Mbps
                if "gbits" in unit or "gbit" in unit:
                    bitrate *= 1000
                elif "kbits" in unit or "kbit" in unit:
                    bitrate /= 1000
                elif "bits/sec" == unit:
                    bitrate /= 1_000_000
                # else: Mbits/sec — already correct

                # Skip the summary line (interval is 0.00-180.00)
                if (end_sec - start_sec) > 5:
                    continue

                entries.append((start_sec, bitrate))
    return entries


def aggregate_iperf3_to_5s(entries):
    """Average 1-second iperf3 entries into 5-second bins to match CSV polling.

    Bins: [0-5), [5-10), ... aligned with CSV ElapsedSeconds 0, 5, 10, ...
    Returns dict: bin_start_sec -> avg_mbps
    """
    if not entries:
        return {}
    bins = {}
    for sec, mbps in entries:
        bin_start = int(sec // 5) * 5
        bins.setdefault(bin_start, []).append(mbps)
    return {k: statistics.mean(v) for k, v in bins.items()}


# ─── Main processing ──────────────────────────────────────────────────────

def detect_phases(rows, idx_elapsed, idx_events, idx_phase):
    """Detect phase boundaries from events column.
    Returns list of (row_index, elapsed, label)."""
    boundaries = []
    for i, r in enumerate(rows):
        events = ""
        if idx_events is not None and idx_events < len(r):
            events = r[idx_events].strip().strip('"')
        if "[phase] Pre-Test" in events:
            boundaries.append((i, float(r[idx_elapsed]), "Pre-Test Idle"))
        elif "[phase] Post-Test" in events:
            boundaries.append((i, float(r[idx_elapsed]), "Post-Test Idle"))
        elif "[phase] Load Test" in events or "[phase] Load" in events:
            boundaries.append((i, float(r[idx_elapsed]), "Load"))
    return boundaries


def process_test(test_def):
    """Process one test scenario. Returns summary dict and time-series rows."""
    device, band, distance, csv_path, speed_txt, label = test_def

    if not os.path.exists(csv_path):
        print(f"  SKIP: {csv_path} not found")
        return None, None

    header, rows = parse_csv(csv_path)
    if not rows:
        print(f"  SKIP: {csv_path} is empty")
        return None, None

    idx_elapsed = find_col(header, "ElapsedSeconds")
    idx_power = find_col(header, "PowerMW")
    idx_tput = find_col(header, "ThroughputTotalMbps")
    idx_iperf3 = find_col(header, "Throughput_iperf3_Mbps")
    idx_phase = find_col(header, "Phase")
    idx_events = find_col(header, "Events")

    # Determine if we have inline throughput (auto) or need external speed file (manual)
    has_inline_tput = idx_iperf3 is not None

    # Parse external speed data if provided
    speed_bins = {}
    if speed_txt and os.path.exists(speed_txt):
        entries = parse_iperf3_txt(speed_txt)
        if entries:
            speed_bins = aggregate_iperf3_to_5s(entries)
        else:
            print(f"  WARNING: {speed_txt} is empty or unparseable")

    # Detect phases
    boundaries = detect_phases(rows, idx_elapsed, idx_events, idx_phase)

    # Build time-series data
    time_series = []
    for r in rows:
        elapsed = float(r[idx_elapsed])
        power = float(r[idx_power])
        phase = r[idx_phase].strip() if idx_phase is not None else ""

        # Get throughput
        if has_inline_tput:
            tput = float(r[idx_iperf3]) if r[idx_iperf3].strip() else 0.0
        elif speed_bins:
            # Align: the CSV load phase starts at ~60s (after 1min pre-test)
            # The iperf3 .txt starts at 0s (runs for 180s = 3 min)
            # So iperf3 second 0 corresponds to CSV elapsed ~60s
            # We need to find the load start time from the CSV
            load_start = 60  # default: 1 min pre-test
            for _, be, bl in boundaries:
                if bl == "Load":
                    load_start = int(be)
                    break
            iperf_sec = int(elapsed) - load_start
            bin_key = int(iperf_sec // 5) * 5
            tput = speed_bins.get(bin_key, 0.0)
        else:
            tput = 0.0

        time_series.append({
            "elapsed": elapsed,
            "power": power,
            "throughput": tput,
            "phase": phase,
        })

    # Compute statistics per phase
    pre_powers = []
    load_powers = []
    load_tputs = []
    post_powers = []

    for ts in time_series:
        p = ts["power"]
        if p <= 0:
            continue  # Skip zero-power glitches
        if ts["phase"] == "pre":
            pre_powers.append(p)
        elif ts["phase"] == "load":
            load_powers.append(p)
            load_tputs.append(ts["throughput"])
        elif ts["phase"] == "post":
            post_powers.append(p)

    idle_mean = statistics.mean(pre_powers) if pre_powers else 0
    idle_std = statistics.stdev(pre_powers) if len(pre_powers) > 1 else 0
    load_mean = statistics.mean(load_powers) if load_powers else 0
    load_std = statistics.stdev(load_powers) if len(load_powers) > 1 else 0
    tput_mean = statistics.mean(load_tputs) if load_tputs else 0
    tput_std = statistics.stdev(load_tputs) if len(load_tputs) > 1 else 0
    post_mean = statistics.mean(post_powers) if post_powers else 0

    power_increase = load_mean - idle_mean
    power_increase_pct = (power_increase / idle_mean * 100) if idle_mean > 0 else 0
    efficiency = tput_mean / (load_mean / 1000) if load_mean > 0 else 0

    summary = {
        "device": device,
        "band": band,
        "distance": distance,
        "label": label,
        "idle_mean": idle_mean,
        "idle_std": idle_std,
        "load_mean": load_mean,
        "load_std": load_std,
        "post_mean": post_mean,
        "tput_mean": tput_mean,
        "tput_std": tput_std,
        "power_increase": power_increase,
        "power_increase_pct": power_increase_pct,
        "efficiency": efficiency,
        "n_pre": len(pre_powers),
        "n_load": len(load_powers),
        "n_post": len(post_powers),
        "has_speed": bool(has_inline_tput or speed_bins),
    }

    return summary, time_series


def main():
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    os.makedirs("data", exist_ok=True)

    print("=" * 74)
    print("Preprocessing Test 4 (Wi-Fi Distance & Band) CSVs for pgfplots...")
    print("=" * 74)

    summaries = []

    for test_def in TESTS:
        device, band, distance, csv_path, speed_txt, label = test_def
        print(f"\n--- {label} ---")
        print(f"  CSV: {csv_path}")
        if speed_txt:
            print(f"  Speed: {speed_txt}")

        summary, time_series = process_test(test_def)
        if summary is None:
            continue

        summaries.append(summary)

        # Print summary
        print(f"  Idle:  {summary['idle_mean']:>8.0f} mW  (±{summary['idle_std']:.0f}, n={summary['n_pre']})")
        print(f"  Load:  {summary['load_mean']:>8.0f} mW  (±{summary['load_std']:.0f}, n={summary['n_load']})")
        print(f"  Post:  {summary['post_mean']:>8.0f} mW  (n={summary['n_post']})")
        print(f"  ΔP:    {summary['power_increase']:>+8.0f} mW  ({summary['power_increase_pct']:+.1f}%)")
        if summary['has_speed']:
            print(f"  Tput:  {summary['tput_mean']:>8.1f} Mbps (±{summary['tput_std']:.1f})")
            print(f"  Eff:   {summary['efficiency']:>8.1f} Mbps/W")
        else:
            print(f"  Tput:  N/A (speed data missing)")

        # Write per-scenario time-series CSV
        scenario_tag = f"{device}_{band}ghz_{distance}"
        ts_path = f"data/test4_{scenario_tag}_data.csv"
        with open(ts_path, "w", newline="") as f:
            f.write("ElapsedSeconds,PowerMW,ThroughputMbps,Phase\n")
            for ts in time_series:
                f.write(f"{ts['elapsed']},{ts['power']},{ts['throughput']:.1f},{ts['phase']}\n")

    # ─── Write summary CSV ──────────────────────────────────────────────
    summary_path = "data/test4_summary.csv"
    with open(summary_path, "w", newline="") as f:
        f.write("Device,Band,Distance,Label,"
                "IdleMeanMW,IdleStdMW,LoadMeanMW,LoadStdMW,PostMeanMW,"
                "ThroughputMeanMbps,ThroughputStdMbps,"
                "PowerIncreaseMW,PowerIncreasePct,"
                "EfficiencyMbpsPerW,HasSpeed\n")
        for s in summaries:
            f.write(f"{s['device']},{s['band']},{s['distance']},{s['label']},"
                    f"{s['idle_mean']:.1f},{s['idle_std']:.1f},"
                    f"{s['load_mean']:.1f},{s['load_std']:.1f},{s['post_mean']:.1f},"
                    f"{s['tput_mean']:.1f},{s['tput_std']:.1f},"
                    f"{s['power_increase']:.1f},{s['power_increase_pct']:.1f},"
                    f"{s['efficiency']:.2f},{1 if s['has_speed'] else 0}\n")

    # ─── Print cross-device comparison table ────────────────────────────
    print("\n" + "=" * 90)
    print("SUMMARY TABLE")
    print("=" * 90)
    print(f"{'Label':<30s} {'Idle':>8s} {'Load':>8s} {'ΔP':>8s} {'Tput':>8s} {'Eff':>8s}")
    print(f"{'':<30s} {'(mW)':>8s} {'(mW)':>8s} {'(mW)':>8s} {'(Mbps)':>8s} {'(M/W)':>8s}")
    print("-" * 90)
    for s in summaries:
        tput_str = f"{s['tput_mean']:>7.0f}" if s['has_speed'] else "    N/A"
        eff_str = f"{s['efficiency']:>7.1f}" if s['has_speed'] else "    N/A"
        print(f"{s['label']:<30s} {s['idle_mean']:>7.0f} {s['load_mean']:>8.0f} "
              f"{s['power_increase']:>+7.0f} {tput_str} {eff_str}")

    # ─── Print key findings ────────────────────────────────────────────
    print("\n" + "=" * 90)
    print("KEY FINDINGS")
    print("=" * 90)

    # Group by device for band comparison
    for dev in ["asus", "fritzbox", "huawei", "alcatel"]:
        dev_tests = [s for s in summaries if s["device"] == dev
                     and s["distance"] in ("near", "wall")]
        if not dev_tests:
            continue
        print(f"\n  {dev.upper()}:")
        for s in dev_tests:
            tstr = f"{s['tput_mean']:.0f} Mbps" if s['has_speed'] else "N/A"
            print(f"    {s['band']:>4s} GHz {s['distance']:<5s}: "
                  f"idle={s['idle_mean']:.0f}, load={s['load_mean']:.0f}, "
                  f"ΔP={s['power_increase']:+.0f} mW, tput={tstr}")

    print(f"\nDone! Files written to data/")
    print(f"  Summary:  {summary_path}")
    print(f"  Time-series: data/test4_*_data.csv")


if __name__ == "__main__":
    main()
