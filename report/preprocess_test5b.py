#!/usr/bin/env python3
"""
Preprocess Test 5B (Bridge Mode) CSV into clean pgfplots-compatible files.
Run from report/ directory:  python3 preprocess_test5b.py

Test 5B: Huawei in bridge mode + Asus RT-AX68U as router (combined power via power strip).
Phase transitions at:
  - 0s: Pre-test baseline (10 min)
  - 600s: Load phase 1 (1 port - Ethernet)
  - 1795s: Load phase 2 (2 ports - +Ethernet 5) -- NOTE: erroneous throughput on port 2
  - 2995s: Load phase 3 (3 ports - +Ethernet 6)
  - 4195s: Load phase 4 (4 ports - +Ethernet 7)
  - 5400s: Post-test baseline (10 min)

Produces:
  data/test5b_data.csv           - ElapsedSeconds,PowerMW,ThroughputTotalMbps,Phase
  data/test5b_data_filtered.csv  - Same but with anomalous throughput values capped
  data/test5b_stats.csv          - per-phase statistics (raw)
  data/test5b_stats_filtered.csv - per-phase statistics (filtered)
  data/test5b_interfaces.csv     - per-interface throughput (raw)
  data/test5b_interfaces_filtered.csv - per-interface throughput (capped)
  data/test5b_phases.csv         - phase boundary timestamps
"""
import csv, statistics, os, re, sys

INPUT_CSV = "../testing/saved_tests/Test5B.csv"

# Maximum plausible throughput per interface (Mbps) -- GbE port max
MAX_IFACE_THROUGHPUT = 1000.0
# Maximum plausible total throughput (Mbps) -- 4 x GbE
MAX_TOTAL_THROUGHPUT = 4000.0

def parse_csv(path):
    rows = []
    header = None
    with open(path, "r", encoding="utf-8-sig") as f:
        for line in f:
            line = line.strip()
            if line.startswith("#") or not line:
                continue
            if header is None:
                header = line.split(",")
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

def process():
    header, rows = parse_csv(INPUT_CSV)

    idx_elapsed = find_col(header, "ElapsedSeconds")
    idx_power   = find_col(header, "PowerMW")
    idx_tput    = find_col(header, "ThroughputTotalMbps")
    idx_phase   = find_col(header, "Phase")
    idx_events  = find_col(header, "Events")

    # Find per-interface throughput columns
    iface_cols = {}
    for i, h in enumerate(header):
        m = re.match(r"Throughput_(.+)_Mbps", h.strip())
        if m:
            iface_name = m.group(1).replace(" ", "_")
            iface_cols[iface_name] = i

    sorted_ifaces = sorted(iface_cols.keys())

    # --- Detect phases from events ---
    phases = []
    for i, r in enumerate(rows):
        events = r[idx_events].strip().strip('"') if idx_events is not None and idx_events < len(r) else ""
        if not events:
            continue
        parts = events.split("|")
        for part in parts:
            part = part.strip()
            if "[phase]" in part or "[iface_start]" in part:
                label = part.replace("[phase]", "").replace("[iface_start]", "").strip()
                if "Interface" in label and "started" in label:
                    m2 = re.search(r"Interface\s+(.+?)\s+started", label)
                    if m2:
                        label = m2.group(1).strip()
                if label:
                    elapsed = float(r[idx_elapsed])
                    phases.append((i, elapsed, label))

    # --- Write phases CSV ---
    with open("data/test5b_phases.csv", "w", newline="") as f:
        f.write("PhaseStartSeconds,PhaseName\n")
        for _, elapsed, label in phases:
            f.write(f"{elapsed},{label.replace(',', ';')}\n")

    # --- 1) Raw data CSV ---
    with open("data/test5b_data.csv", "w", newline="") as f:
        f.write("ElapsedSeconds,PowerMW,ThroughputTotalMbps,Phase\n")
        for r in rows:
            elapsed = float(r[idx_elapsed])
            power   = float(r[idx_power])
            tput    = float(r[idx_tput])
            phase   = r[idx_phase].strip()
            f.write(f"{elapsed},{power},{tput},{phase}\n")

    # --- 2) Filtered data CSV (cap throughput) ---
    with open("data/test5b_data_filtered.csv", "w", newline="") as f:
        f.write("ElapsedSeconds,PowerMW,ThroughputTotalMbps,Phase\n")
        for r in rows:
            elapsed = float(r[idx_elapsed])
            power   = float(r[idx_power])
            tput    = float(r[idx_tput])
            phase   = r[idx_phase].strip()
            # Cap total throughput
            capped_total = 0.0
            for iname in sorted_ifaces:
                raw_val = float(r[iface_cols[iname]])
                capped_total += min(raw_val, MAX_IFACE_THROUGHPUT)
            if tput > MAX_TOTAL_THROUGHPUT:
                tput = capped_total
            f.write(f"{elapsed},{power},{tput:.2f},{phase}\n")

    # --- 3) Raw interfaces CSV ---
    with open("data/test5b_interfaces.csv", "w", newline="") as f:
        hdr = "ElapsedSeconds," + ",".join(f"Throughput_{n}_Mbps" for n in sorted_ifaces)
        f.write(hdr + "\n")
        for r in rows:
            elapsed = r[idx_elapsed]
            vals = [r[iface_cols[n]] for n in sorted_ifaces]
            f.write(elapsed + "," + ",".join(vals) + "\n")

    # --- 4) Filtered interfaces CSV ---
    with open("data/test5b_interfaces_filtered.csv", "w", newline="") as f:
        hdr = "ElapsedSeconds," + ",".join(f"Throughput_{n}_Mbps" for n in sorted_ifaces)
        f.write(hdr + "\n")
        for r in rows:
            elapsed = r[idx_elapsed]
            vals = []
            for n in sorted_ifaces:
                raw_val = float(r[iface_cols[n]])
                vals.append(f"{min(raw_val, MAX_IFACE_THROUGHPUT):.2f}")
            f.write(elapsed + "," + ",".join(vals) + "\n")

    # --- 5) Per-phase statistics (raw and filtered) ---
    for suffix, use_filter in [("", False), ("_filtered", True)]:
        phase_data = {}
        for pi in range(len(phases)):
            start_idx = phases[pi][0]
            end_idx = phases[pi+1][0] if pi+1 < len(phases) else len(rows)
            pname = phases[pi][2]
            start_time = phases[pi][1]
            end_time = float(rows[end_idx-1][idx_elapsed]) if end_idx <= len(rows) else float(rows[-1][idx_elapsed])

            powers = []
            tputs = []
            iface_tputs = {n: [] for n in sorted_ifaces}

            for j in range(start_idx, end_idx):
                powers.append(float(rows[j][idx_power]))
                raw_tput = float(rows[j][idx_tput])

                if use_filter:
                    # Cap per-interface then sum
                    capped = 0.0
                    for n in sorted_ifaces:
                        val = float(rows[j][iface_cols[n]])
                        capped_val = min(val, MAX_IFACE_THROUGHPUT)
                        capped += capped_val
                        iface_tputs[n].append(capped_val)
                    tputs.append(capped if raw_tput > MAX_TOTAL_THROUGHPUT else raw_tput)
                else:
                    tputs.append(raw_tput)
                    for n in sorted_ifaces:
                        iface_tputs[n].append(float(rows[j][iface_cols[n]]))

            if len(powers) < 2:
                continue

            avg_p = statistics.mean(powers)
            std_p = statistics.stdev(powers) if len(powers) > 1 else 0
            avg_t = statistics.mean(tputs)
            std_t = statistics.stdev(tputs) if len(tputs) > 1 else 0
            eff = avg_t / (avg_p / 1000) if avg_p > 0 else 0
            duration = end_time - start_time

            phase_data[pname] = {
                "avg_power": avg_p, "std_power": std_p,
                "min_power": min(powers), "max_power": max(powers),
                "avg_tput": avg_t, "std_tput": std_t,
                "duration": duration, "count": len(powers),
                "efficiency": eff,
                "iface_tputs": {n: statistics.mean(iface_tputs[n]) if iface_tputs[n] else 0 for n in sorted_ifaces}
            }

        with open(f"data/test5b_stats{suffix}.csv", "w", newline="") as f:
            f.write("Phase,AvgPowerMW,StdDevPowerMW,MinPowerMW,MaxPowerMW,AvgThroughputMbps,StdDevThroughputMbps,DurationSeconds,DataPoints,EfficiencyMbpsPerW")
            for n in sorted_ifaces:
                f.write(f",AvgTput_{n}_Mbps")
            f.write("\n")
            for pname, s in phase_data.items():
                f.write(f"{pname.replace(',',';')},{s['avg_power']:.1f},{s['std_power']:.1f},{s['min_power']:.1f},{s['max_power']:.1f},{s['avg_tput']:.1f},{s['std_tput']:.1f},{s['duration']:.0f},{s['count']},{s['efficiency']:.2f}")
                for n in sorted_ifaces:
                    f.write(f",{s['iface_tputs'][n]:.1f}")
                f.write("\n")

    # --- Print summary ---
    print("=" * 80)
    print("TEST 5B: ISP Bridge Mode (Huawei bridge + Asus router) - Combined Power")
    print("=" * 80)
    print(f"\nTotal data points: {len(rows)}")
    print(f"Phases detected: {len(phases)}")
    print(f"Interfaces: {', '.join(sorted_ifaces)}")
    print()

    # Read filtered stats for summary
    print("--- RAW Statistics ---")
    for pi in range(len(phases)):
        start_idx = phases[pi][0]
        end_idx = phases[pi+1][0] if pi+1 < len(phases) else len(rows)
        pname = phases[pi][2]
        powers = [float(rows[j][idx_power]) for j in range(start_idx, end_idx)]
        tputs = [float(rows[j][idx_tput]) for j in range(start_idx, end_idx)]
        avg_p = statistics.mean(powers) if powers else 0
        avg_t = statistics.mean(tputs) if tputs else 0
        print(f"  {pname:25s}  avg_power={avg_p:.0f} mW  avg_tput={avg_t:.0f} Mbps  n={len(powers)}")

    print()
    print("--- FILTERED Statistics (throughput capped at 1000 Mbps/iface) ---")
    for pi in range(len(phases)):
        start_idx = phases[pi][0]
        end_idx = phases[pi+1][0] if pi+1 < len(phases) else len(rows)
        pname = phases[pi][2]
        powers = [float(rows[j][idx_power]) for j in range(start_idx, end_idx)]

        filtered_tputs = []
        for j in range(start_idx, end_idx):
            raw_tput = float(rows[j][idx_tput])
            if raw_tput > MAX_TOTAL_THROUGHPUT:
                capped = sum(min(float(rows[j][iface_cols[n]]), MAX_IFACE_THROUGHPUT) for n in sorted_ifaces)
                filtered_tputs.append(capped)
            else:
                filtered_tputs.append(raw_tput)

        avg_p = statistics.mean(powers) if powers else 0
        avg_t = statistics.mean(filtered_tputs) if filtered_tputs else 0
        print(f"  {pname:25s}  avg_power={avg_p:.0f} mW  avg_tput={avg_t:.0f} Mbps  n={len(powers)}")

    # --- Comparison with Huawei standalone ---
    print()
    print("--- COMPARISON: Huawei Standalone (Test 1) vs Bridge+Asus (Test 5B) ---")
    print("  Phase               Huawei Alone (mW)    Bridge+Asus (mW)    Delta")
    huawei_standalone = {
        "Idle":    8962,
        "1 Port":  9231,
        "2 Ports": 9409,
        "3 Ports": 9494,
        "4 Ports": 9510,
        "Post":    8986,
    }
    # Calculate Test5B values
    test5b_phases_power = {}
    for pi in range(len(phases)):
        start_idx = phases[pi][0]
        end_idx = phases[pi+1][0] if pi+1 < len(phases) else len(rows)
        pname = phases[pi][2]
        powers = [float(rows[j][idx_power]) for j in range(start_idx, end_idx)]
        if powers:
            test5b_phases_power[pname] = statistics.mean(powers)

    mapping = [
        ("Idle", "Pre-Test Baseline"),
        ("1 Port", "Ethernet"),
        ("2 Ports", "Ethernet 5"),
        ("3 Ports", "Ethernet 6"),
        ("4 Ports", "Ethernet 7"),
        ("Post", "Post-Test Baseline"),
    ]
    for hw_name, t5b_name in mapping:
        hw_val = huawei_standalone.get(hw_name, 0)
        t5b_val = test5b_phases_power.get(t5b_name, 0)
        if t5b_val == 0:
            continue
        delta = t5b_val - hw_val
        pct = (delta / hw_val * 100) if hw_val > 0 else 0
        print(f"  {hw_name:15s}  {hw_val:>8.0f}            {t5b_val:>8.0f}            {delta:+.0f} ({pct:+.1f}%)")

    # Anomaly analysis
    print()
    print("--- THROUGHPUT ANOMALY ANALYSIS ---")
    for pi in range(len(phases)):
        start_idx = phases[pi][0]
        end_idx = phases[pi+1][0] if pi+1 < len(phases) else len(rows)
        pname = phases[pi][2]
        anomaly_count = 0
        total_count = 0
        for j in range(start_idx, end_idx):
            total_count += 1
            for n in sorted_ifaces:
                val = float(rows[j][iface_cols[n]])
                if val > MAX_IFACE_THROUGHPUT:
                    anomaly_count += 1
                    break
        pct = (anomaly_count / total_count * 100) if total_count > 0 else 0
        if anomaly_count > 0:
            print(f"  {pname:25s}: {anomaly_count}/{total_count} data points ({pct:.1f}%) have throughput > 1000 Mbps")


if __name__ == "__main__":
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    os.makedirs("data", exist_ok=True)
    print("Preprocessing Test 5B CSV for pgfplots...")
    if os.path.exists(INPUT_CSV):
        process()
    else:
        print(f"  SKIP: {INPUT_CSV} not found")
    print("\nDone! Files in data/")
