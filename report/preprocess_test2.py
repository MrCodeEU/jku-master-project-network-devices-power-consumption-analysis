#!/usr/bin/env python3
"""
Preprocess Test 2 (Throughput Scaling) CSVs into clean pgfplots-compatible files.
Run from report/ directory:  python3 preprocess_test2.py

Test 2: Single-port ramp-step throughput scaling test.
  - Huawei, Fritzbox: Quick rerun (5m load, 1m pre/post) — stretched 5x to match full-length tests
  - Asus: Full 25m run (5m pre/post) at 1 GbE
  - Alcatel: Full 25m run at 100 Mbps (5m pre/post)

Produces per device:
  data/test2_<device>_data.csv    - ElapsedSeconds,PowerMW,ThroughputMbps,TargetMbps,Phase
  data/test2_<device>_stats.csv   - per-phase statistics
  data/test2_<device>_phases.csv  - phase boundary timestamps
"""
import csv, statistics, os, re

DEVICES = {
    "huawei":   {"file": "../testing/saved_tests/test2/huawei.csv",   "stretch": 5},
    "fritzbox": {"file": "../testing/saved_tests/test2/fritzbox.csv", "stretch": 5},
    "asus":     {"file": "../testing/saved_tests/test2/asus.csv",     "stretch": 1},
    "alcatel":  {"file": "../testing/saved_tests/test2/alcatel.csv",  "stretch": 1},
}


def parse_csv(path):
    """Parse CSV skipping comment lines starting with #."""
    rows = []
    header = None
    with open(path, "r", encoding="utf-8-sig") as f:
        for line in f:
            line = line.strip()
            if line.startswith("#") or not line:
                continue
            if header is None:
                header = [h.strip() for h in line.split(",")]
                continue
            reader = csv.reader([line])
            for parsed in reader:
                rows.append(parsed)
    return header, rows


def find_col(header, name):
    name_l = name.lower()
    for i, h in enumerate(header):
        if h.lower().strip() == name_l:
            return i
    return None


def stretch_rows(rows, idx_elapsed, idx_events, factor):
    """Duplicate each row `factor` times, adjusting elapsed seconds.

    Original: 84 points at 5s intervals (0, 5, 10, ..., 415)
    Stretched: 420 points at 5s intervals (0, 5, 10, ..., 2095)
    Each original point i becomes 5 new points at indices i*factor..i*factor+(factor-1).
    """
    if factor <= 1:
        return rows

    stretched = []
    for i, row in enumerate(rows):
        for rep in range(factor):
            new_row = list(row)
            new_elapsed = (i * factor + rep) * 5
            new_row[idx_elapsed] = str(new_elapsed)
            # Only keep events on first replica
            if rep > 0 and idx_events is not None:
                new_row[idx_events] = '""'
            stretched.append(new_row)
    return stretched


def detect_boundaries(rows, idx_elapsed, idx_events):
    """Detect phase boundaries from events column.

    Returns list of (row_index, elapsed_seconds, label).
    """
    boundaries = []
    for i, r in enumerate(rows):
        events = r[idx_events].strip().strip('"') if idx_events is not None and idx_events < len(r) else ""
        if not events:
            continue
        parts = [p.strip() for p in events.split("|")]
        for part in parts:
            if "[phase] Pre-Test" in part:
                boundaries.append((i, float(r[idx_elapsed]), "Pre-Test Idle"))
            elif "[phase] Post-Test" in part:
                boundaries.append((i, float(r[idx_elapsed]), "Post-Test Idle"))
            elif "[phase] Load Test" in part:
                pass  # The co-occurring [ramp] event handles load start
            elif "[ramp]" in part:
                m = re.search(r'Ramp (\d+)/(\d+):\s*([\d.]+)\s*Mbps', part)
                if m:
                    step = int(m.group(1))
                    target = float(m.group(3))
                    boundaries.append((i, float(r[idx_elapsed]),
                                       f"Step {step}: {target:.0f} Mbps"))
                elif "complete" in part.lower():
                    m2 = re.search(r'Ramp complete:\s*([\d.]+)\s*Mbps', part)
                    if m2:
                        target = float(m2.group(1))
                        boundaries.append((i, float(r[idx_elapsed]),
                                           f"Sustained {target:.0f} Mbps"))
    return boundaries


def compute_phase_stats(rows, boundaries, idx_power, idx_tput, idx_target, idx_elapsed):
    """Compute per-phase statistics between boundaries."""
    stats = []
    for pi in range(len(boundaries)):
        start_idx = boundaries[pi][0]
        end_idx = boundaries[pi + 1][0] if pi + 1 < len(boundaries) else len(rows)
        pname = boundaries[pi][2]
        start_time = boundaries[pi][1]

        powers = []
        tputs = []
        targets = []

        for j in range(start_idx, end_idx):
            p = float(rows[j][idx_power])
            # Skip zero-power glitches (DECT 200 communication errors)
            if p <= 0:
                continue
            powers.append(p)
            tputs.append(float(rows[j][idx_tput]))
            targets.append(float(rows[j][idx_target]))

        if len(powers) < 1:
            continue

        avg_p = statistics.mean(powers)
        std_p = statistics.stdev(powers) if len(powers) > 1 else 0
        avg_t = statistics.mean(tputs)
        std_t = statistics.stdev(tputs) if len(tputs) > 1 else 0
        avg_target = statistics.mean(targets)
        end_time = float(rows[min(end_idx, len(rows)) - 1][idx_elapsed])
        duration = end_time - start_time
        eff = avg_t / (avg_p / 1000) if avg_p > 0 else 0

        stats.append({
            "name": pname, "avg_power": avg_p, "std_power": std_p,
            "min_power": min(powers), "max_power": max(powers),
            "avg_tput": avg_t, "std_tput": std_t,
            "avg_target": avg_target,
            "duration": duration, "count": len(powers),
            "efficiency": eff,
        })
    return stats


def process_device(device_name, cfg):
    path = cfg["file"]
    stretch = cfg["stretch"]

    if not os.path.exists(path):
        print(f"  SKIP: {path} not found")
        return

    header, rows = parse_csv(path)

    idx_elapsed  = find_col(header, "ElapsedSeconds")
    idx_power    = find_col(header, "PowerMW")
    idx_tput     = find_col(header, "ThroughputTotalMbps")
    idx_target   = find_col(header, "TargetThroughputTotalMbps")
    idx_phase    = find_col(header, "Phase")
    idx_events   = find_col(header, "Events")

    print(f"\n--- {device_name.upper()} ---")
    print(f"  Raw data points: {len(rows)}")

    # Stretch short runs
    if stretch > 1:
        rows = stretch_rows(rows, idx_elapsed, idx_events, stretch)
        print(f"  After {stretch}x stretch: {len(rows)} data points")

    # Detect phase boundaries
    boundaries = detect_boundaries(rows, idx_elapsed, idx_events)
    print(f"  Phase boundaries: {len(boundaries)}")
    for _, elapsed, label in boundaries:
        print(f"    {elapsed:>7.0f}s  {label}")

    prefix = f"data/test2_{device_name}"

    # --- Write phases CSV ---
    with open(f"{prefix}_phases.csv", "w", newline="") as f:
        f.write("PhaseStartSeconds,PhaseName\n")
        for _, elapsed, label in boundaries:
            f.write(f"{elapsed},{label}\n")

    # --- Write data CSV ---
    with open(f"{prefix}_data.csv", "w", newline="") as f:
        f.write("ElapsedSeconds,PowerMW,ThroughputMbps,TargetMbps,Phase\n")
        for r in rows:
            elapsed = float(r[idx_elapsed])
            power   = float(r[idx_power])
            tput    = float(r[idx_tput])
            target  = float(r[idx_target])
            phase   = r[idx_phase].strip()
            f.write(f"{elapsed},{power},{tput:.2f},{target:.0f},{phase}\n")

    # --- Compute and write stats ---
    stats = compute_phase_stats(rows, boundaries, idx_power, idx_tput,
                                idx_target, idx_elapsed)

    with open(f"{prefix}_stats.csv", "w", newline="") as f:
        f.write("Phase,AvgPowerMW,StdDevPowerMW,MinPowerMW,MaxPowerMW,"
                "AvgThroughputMbps,StdDevThroughputMbps,AvgTargetMbps,"
                "DurationSeconds,DataPoints,EfficiencyMbpsPerW\n")
        for s in stats:
            f.write(f"{s['name']},{s['avg_power']:.1f},{s['std_power']:.1f},"
                    f"{s['min_power']:.1f},{s['max_power']:.1f},"
                    f"{s['avg_tput']:.1f},{s['std_tput']:.1f},"
                    f"{s['avg_target']:.0f},{s['duration']:.0f},"
                    f"{s['count']},{s['efficiency']:.2f}\n")

    # Print summary table
    print(f"\n  {'Phase':<28s} {'AvgPower':>9s} {'StdDev':>7s} {'AvgTput':>8s} "
          f"{'Target':>7s} {'Eff':>6s} {'N':>4s}")
    print(f"  {'-'*28} {'-'*9} {'-'*7} {'-'*8} {'-'*7} {'-'*6} {'-'*4}")
    for s in stats:
        print(f"  {s['name']:<28s} {s['avg_power']:>8.0f}W {s['std_power']:>6.1f} "
              f"{s['avg_tput']:>7.0f}M {s['avg_target']:>6.0f}M "
              f"{s['efficiency']:>5.1f} {s['count']:>4d}")

    return stats


def print_comparison(all_stats):
    """Print cross-device comparison for matching ramp steps."""
    print("\n" + "=" * 90)
    print("CROSS-DEVICE COMPARISON (1 GbE devices)")
    print("=" * 90)

    # Map step names to stats per device
    gbe_devices = ["huawei", "fritzbox", "asus"]
    step_names = ["Pre-Test Idle", "Step 1: 250 Mbps", "Step 2: 500 Mbps",
                  "Step 3: 750 Mbps", "Step 4: 1000 Mbps", "Sustained 1000 Mbps",
                  "Post-Test Idle"]

    print(f"\n  {'Phase':<22s}", end="")
    for dev in gbe_devices:
        print(f"  {dev:>12s}", end="")
    print()
    print(f"  {'-'*22}", end="")
    for _ in gbe_devices:
        print(f"  {'-'*12}", end="")
    print()

    for step in step_names:
        print(f"  {step:<22s}", end="")
        for dev in gbe_devices:
            dev_stats = all_stats.get(dev, [])
            found = [s for s in dev_stats if s["name"] == step]
            if found:
                print(f"  {found[0]['avg_power']:>8.0f} mW", end="")
            else:
                print(f"  {'---':>12s}", end="")
        print()


if __name__ == "__main__":
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    os.makedirs("data", exist_ok=True)
    print("=" * 70)
    print("Preprocessing Test 2 CSVs for pgfplots...")
    print("=" * 70)

    all_stats = {}
    for device_name, cfg in DEVICES.items():
        stats = process_device(device_name, cfg)
        if stats:
            all_stats[device_name] = stats

    print_comparison(all_stats)
    print("\nDone! Files written to data/")
