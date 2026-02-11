#!/usr/bin/env python3
"""
Preprocess raw test CSVs into clean pgfplots-compatible files.
Run from report/ directory:  python3 preprocess_csvs.py

Produces for each device:
  data/{device}_data.csv       - ElapsedSeconds,PowerMW,ThroughputTotalMbps,Phase
  data/{device}_stats.csv      - per-phase statistics
  data/{device}_phases.csv     - phase boundary timestamps
  data/{device}_interfaces.csv - per-interface throughput
"""
import csv, statistics, os, re, sys

TESTS = {
    "fritzbox": "../testing/saved_tests/first_4_port_test_fritzbox_7530.csv",
    "huawei":   "../testing/saved_tests/first_4_port_test_huawei.csv",
    "asus":     "../testing/saved_tests/first_4_port_test_asus.csv",
    "alcatel":  "../testing/saved_tests/first_4_port_test_alcatel.csv",
}

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
                # Find column indices
                continue
            # Smart CSV parse (events field may contain commas inside quotes)
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

def process_device(name, path):
    header, rows = parse_csv(path)

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

    # --- 1) data CSV ---
    data_rows = []
    for r in rows:
        elapsed = float(r[idx_elapsed])
        power   = float(r[idx_power])
        tput    = float(r[idx_tput])
        phase   = r[idx_phase].strip()
        data_rows.append((elapsed, power, tput, phase))

    with open(f"data/{name}_data.csv", "w", newline="") as f:
        f.write("ElapsedSeconds,PowerMW,ThroughputTotalMbps,Phase\n")
        for elapsed, power, tput, phase in data_rows:
            f.write(f"{elapsed},{power},{tput},{phase}\n")

    # --- 2) Detect phases from events ---
    phases = []  # list of (start_idx, start_time, phase_name)
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

    # Write phases CSV
    with open(f"data/{name}_phases.csv", "w", newline="") as f:
        f.write("PhaseStartSeconds,PhaseName\n")
        for _, elapsed, label in phases:
            f.write(f"{elapsed},{label.replace(',', ';')}\n")

    # --- 3) Per-phase statistics ---
    phase_data = {}  # phase_name -> list of (power, tput)
    for pi in range(len(phases)):
        start_idx = phases[pi][0]
        end_idx = phases[pi+1][0] if pi+1 < len(phases) else len(rows)
        pname = phases[pi][2]
        start_time = phases[pi][1]
        end_time = float(rows[end_idx-1][idx_elapsed]) if end_idx <= len(rows) else float(rows[-1][idx_elapsed])

        powers = []
        tputs = []
        for j in range(start_idx, end_idx):
            powers.append(float(rows[j][idx_power]))
            tputs.append(float(rows[j][idx_tput]))

        if not powers:
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
            "efficiency": eff
        }

    with open(f"data/{name}_stats.csv", "w", newline="") as f:
        f.write("Phase,AvgPowerMW,StdDevPowerMW,MinPowerMW,MaxPowerMW,AvgThroughputMbps,StdDevThroughputMbps,DurationSeconds,DataPoints,EfficiencyMbpsPerW\n")
        for pname, s in phase_data.items():
            f.write(f"{pname.replace(',',';')},{s['avg_power']:.1f},{s['std_power']:.1f},{s['min_power']:.1f},{s['max_power']:.1f},{s['avg_tput']:.1f},{s['std_tput']:.1f},{s['duration']:.0f},{s['count']},{s['efficiency']:.2f}\n")

    # --- 4) Per-interface throughput CSV ---
    if iface_cols:
        sorted_ifaces = sorted(iface_cols.keys())
        with open(f"data/{name}_interfaces.csv", "w", newline="") as f:
            hdr = "ElapsedSeconds," + ",".join(f"Throughput_{n}_Mbps" for n in sorted_ifaces)
            f.write(hdr + "\n")
            for r in rows:
                elapsed = r[idx_elapsed]
                vals = [r[iface_cols[n]] for n in sorted_ifaces]
                f.write(elapsed + "," + ",".join(vals) + "\n")

    print(f"  {name}: {len(data_rows)} data points, {len(phases)} phases, {len(iface_cols)} interfaces")
    for pname, s in phase_data.items():
        print(f"    {pname:30s}  avg={s['avg_power']:.0f} mW  std={s['std_power']:.0f}  tput={s['avg_tput']:.0f} Mbps  dur={s['duration']:.0f}s  n={s['count']}")

if __name__ == "__main__":
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    os.makedirs("data", exist_ok=True)
    print("Preprocessing test CSVs for pgfplots...")
    for name, path in TESTS.items():
        if os.path.exists(path):
            process_device(name, path)
        else:
            print(f"  SKIP {name}: {path} not found")
    print("Done! Files in data/")
