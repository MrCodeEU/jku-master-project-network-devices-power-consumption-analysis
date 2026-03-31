#!/usr/bin/env python3
"""
Preprocess Test 3 (Incremental Subsystem Power Profiling) CSVs.
Run from report/ directory:  python3 preprocess_test3.py

Test 3: Group-based subsystem profiling with manual intervention between groups.
Each group is a separate CSV file. Custom events mark sub-phase transitions
(e.g., cable additions within Group 1).

Produces:
  data/test3_<device>_summary.csv      - per-sub-phase power statistics (Run A)
  data/test3_<device>_eco_summary.csv   - per-sub-phase power statistics (Run B / eco)
  data/test3_comparison.csv             - cross-device comparison table
"""
import csv, statistics, os, re, sys

BASE = "../testing/saved_tests/test3"

# Device configurations: maps group numbers to CSV filenames
# Group 1_and_2 = bare min + cables (+ USB for some)
# Group 3 = USB (or WiFi for some)
# Group 4 = WiFi radios / clients
# Group 5 = WiFi load
# Group 6 = Full load (WiFi + Ethernet)
DEVICES = {
    "asus": {
        "run_a": {
            "1_and_2": "asus_1_and_2.csv",
            "3": "asus_3.csv",
            "4": "asus_4.csv",
            "5": "asus_5.csv",
            "6": "asus_6.csv",
        },
        "run_b": {
            "3": "asus_eco_3.csv",
            "4": "asus_eco_4.csv",
            "5": "asus_eco_5.csv",
            "6": "asus_eco_6.csv",
        },
        "lan_ports": 4,
        "usb_ports": 2,
        "has_5ghz": True,
        "rated_max_w": 33,
    },
    "huawei": {
        "run_a": {
            "1_and_2": "huawei_1_and_2.csv",
            "3": "huawei_3.csv",
            "4": "huawei_4.csv",
            "5": "huawei_5.csv",
            "6": "huawei_6.csv",
        },
        "run_b": {
            "3": "huawei_eco_3.csv",
            "4": "huawei_eco_4.csv",
            "5": "huawei_eco_5.csv",
            "6": "huawei_eco_6.csv",
        },
        "lan_ports": 4,
        "usb_ports": 1,
        "has_5ghz": True,
        "rated_max_w": 24,
    },
    "fritzbox": {
        "run_a": {
            "1_and_2": "fritzbox_1_and_2.csv",
            "3": "fritzbox_3.csv",
            "4": "fritzbox_4.csv",
            "5": "fritzbox_5.csv",
            "6": "fritzbox_6.csv",
        },
        "run_b": {
            "1_and_2": "fritzbox_eco_1_and_2.csv",
            "3": "fritzbox_eco_3.csv",
            "4": "fritzbox_eco_4.csv",
            "5": "fritzbox_eco_5.csv",
            "6": "fritzbox_eco_6.csv",
        },
        "lan_ports": 4,
        "usb_ports": 1,
        "has_5ghz": True,
        "rated_max_w": 18,
        "eth7_anomaly": True,  # Ethernet 7 reports inflated throughput
    },
    "alcatel": {
        "run_a": {
            "1_and_2": "alcatel_1.csv",  # named differently
            "3": "alcatel_3.csv",        # actually USB skip -> WiFi
            "4": "alcatel_4.csv",
            "5": "alcatel_5.csv",
            "6": "alcatel_6.csv",
        },
        "run_b": None,  # No eco mode available
        "lan_ports": 2,
        "usb_ports": 0,
        "has_5ghz": False,
        "rated_max_w": 10,
    },
}

# Manual WiFi throughput observations from test notes (Mbps)
WIFI_NOTES = {
    "asus":     {"group5_wifi_mbps": 147, "group6_wifi_mbps": 140},
    "huawei":   {"group5_wifi_mbps": 58.5, "group6_wifi_mbps": 70},
    "fritzbox": {"group5_wifi_mbps": 194, "group6_wifi_mbps": 163},
    "alcatel":  {"group5_wifi_mbps": 17.5, "group6_wifi_mbps": 8.45},
}
WIFI_NOTES_ECO = {
    "asus":     {"group5_wifi_mbps": 163, "group6_wifi_mbps": 192},
    "huawei":   {"group5_wifi_mbps": 98.3, "group6_wifi_mbps": 95.4},
    "fritzbox": {"group5_wifi_mbps": 185, "group6_wifi_mbps": 180},
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


def get_power_values(rows, idx_power, start_idx=0, end_idx=None):
    """Extract non-zero power values from a range of rows."""
    if end_idx is None:
        end_idx = len(rows)
    powers = []
    for i in range(start_idx, end_idx):
        p = float(rows[i][idx_power])
        if p > 0:
            powers.append(p)
    return powers


def compute_stats(values):
    """Compute mean, std, min, max for a list of values."""
    if not values:
        return {"mean": 0, "std": 0, "min": 0, "max": 0, "count": 0}
    return {
        "mean": statistics.mean(values),
        "std": statistics.stdev(values) if len(values) > 1 else 0,
        "min": min(values),
        "max": max(values),
        "count": len(values),
    }


def find_custom_events(rows, idx_events):
    """Find all rows with custom events, return list of (row_index, event_text).

    Handles both formats:
      - "[custom] lan +1"  (standard app format)
      - "lan +1"           (older/manual format without [custom] prefix)
    """
    events = []
    if idx_events is None:
        return events
    # Standard phase/system events to exclude
    system_events = {"Pre-Test Baseline", "Load Test", "Post-Test Baseline"}
    for i, r in enumerate(rows):
        ev = r[idx_events].strip().strip('"') if idx_events < len(r) else ""
        if not ev:
            continue
        parts = [p.strip() for p in ev.split("|")]
        for part in parts:
            if "[custom]" in part:
                text = part.replace("[custom]", "").strip()
                events.append((i, text))
            elif "[phase]" in part or "[iface_start]" in part or "[iface_stop]" in part or "[ramp]" in part:
                continue  # skip system events
            elif part in system_events:
                continue
            elif part:
                # Bare event text (Alcatel format)
                events.append((i, part))
    return events


def find_phase_boundary(rows, idx_events, phase_name="Load Test"):
    """Find the row index where a phase starts."""
    if idx_events is None:
        return 0
    for i, r in enumerate(rows):
        ev = r[idx_events].strip().strip('"') if idx_events < len(r) else ""
        if f"[phase] {phase_name}" in ev or phase_name in ev:
            return i
    return 0


def process_group_1_and_2(path, device_name, cfg):
    """Process Group 1+2: bare minimum + incremental cable additions (+ USB)."""
    header, rows = parse_csv(path)
    idx_power = find_col(header, "PowerMW")
    idx_events = find_col(header, "Events")
    idx_elapsed = find_col(header, "ElapsedSeconds")

    custom_events = find_custom_events(rows, idx_events)
    load_start = find_phase_boundary(rows, idx_events)

    results = []

    # Pre-test = bare minimum (no cables, no WiFi)
    pre_powers = get_power_values(rows, idx_power, 0, load_start)
    results.append({
        "phase": "Bare Minimum",
        "group": "1",
        **compute_stats(pre_powers),
    })

    # Parse sub-phases from custom events
    # Events like "lan +1", "lan +2", "usb +1", etc.
    cable_events = []
    usb_events = []
    for idx, text in custom_events:
        text_l = text.lower()
        if "lan" in text_l or "cable" in text_l or "eth" in text_l:
            cable_events.append(idx)
        elif "usb" in text_l:
            usb_events.append(idx)

    # Build sub-phase boundaries: each event starts a new sub-phase
    all_boundaries = sorted(set([load_start] + cable_events + usb_events))

    cable_count = 0
    usb_count = 0
    for bi in range(len(all_boundaries)):
        start = all_boundaries[bi]
        end = all_boundaries[bi + 1] if bi + 1 < len(all_boundaries) else len(rows)

        # Skip the first boundary if it's just the load phase marker
        if start == load_start and start not in cable_events and start not in usb_events:
            # This is the load start before any cables - usually very short, skip
            if bi + 1 < len(all_boundaries) and all_boundaries[bi + 1] - start <= 2:
                continue
            # Otherwise treat as brief pre-cable period
            powers = get_power_values(rows, idx_power, start, end)
            if powers:
                results.append({
                    "phase": "Load Start (no change)",
                    "group": "1",
                    **compute_stats(powers),
                })
            continue

        # Determine what this sub-phase is
        if start in cable_events:
            cable_count += 1
            phase_name = f"+Cable {cable_count}"
            group = "1"
        elif start in usb_events:
            usb_count += 1
            phase_name = f"+USB {usb_count}"
            group = "2"
        else:
            continue

        # Use data points AFTER the event (skip first 2 points for stabilization)
        stable_start = min(start + 2, end)
        powers = get_power_values(rows, idx_power, stable_start, end)
        if powers:
            results.append({
                "phase": phase_name,
                "group": group,
                **compute_stats(powers),
            })

    return results


def process_simple_group(path, group_num, phase_name):
    """Process a simple group file: extract pre-baseline and load-phase power."""
    if not os.path.exists(path):
        return []

    header, rows = parse_csv(path)
    idx_power = find_col(header, "PowerMW")
    idx_events = find_col(header, "Events")

    load_start = find_phase_boundary(rows, idx_events)

    results = []

    # The pre-baseline of this group = state after previous group
    # (useful for continuity check but we mainly care about load phase)

    # Parse custom events within load phase for sub-phases
    custom_events = find_custom_events(rows, idx_events)

    if custom_events:
        # Build sub-phases from custom events
        boundaries = [(load_start, "pre-change")]
        for idx, text in custom_events:
            boundaries.append((idx, text))

        for bi in range(len(boundaries)):
            start = boundaries[bi][0]
            end = boundaries[bi + 1][0] if bi + 1 < len(boundaries) else len(rows)
            label = boundaries[bi][1]

            if label == "pre-change":
                # Brief period before first change - skip if very short
                if end - start <= 2:
                    continue
                powers = get_power_values(rows, idx_power, start, end)
                if powers:
                    results.append({
                        "phase": f"G{group_num} Pre-change",
                        "group": str(group_num),
                        **compute_stats(powers),
                    })
            else:
                # After event: skip 2 points for stabilization
                stable_start = min(start + 2, end)
                powers = get_power_values(rows, idx_power, stable_start, end)
                if powers:
                    results.append({
                        "phase": f"+{label}",
                        "group": str(group_num),
                        **compute_stats(powers),
                    })
    else:
        # No custom events: just use entire load phase
        # Skip first 2 points after load start for stabilization
        stable_start = min(load_start + 2, len(rows))
        powers = get_power_values(rows, idx_power, stable_start)
        if powers:
            results.append({
                "phase": phase_name,
                "group": str(group_num),
                **compute_stats(powers),
            })

    return results


def process_group_6(path, device_name, cfg):
    """Process Group 6: full load (WiFi + Ethernet).

    For Fritzbox: handle Ethernet 7 anomaly by interpolating throughput
    from the 3 working ports.
    """
    if not os.path.exists(path):
        return []

    header, rows = parse_csv(path)
    idx_power = find_col(header, "PowerMW")
    idx_tput = find_col(header, "ThroughputTotalMbps")
    idx_events = find_col(header, "Events")

    load_start = find_phase_boundary(rows, idx_events)
    # Skip first 2 points for stabilization
    stable_start = min(load_start + 2, len(rows))

    powers = get_power_values(rows, idx_power, stable_start)

    # Compute throughput, handling Fritzbox anomaly
    tputs = []
    corrected_tputs = []
    has_anomaly = cfg.get("eth7_anomaly", False)

    if has_anomaly and idx_tput is not None:
        # Find all per-interface Throughput columns (excluding Tailscale)
        eth_cols = []
        for i, h in enumerate(header):
            if h.startswith("Throughput_") and h.endswith("_Mbps") and "Tailscale" not in h:
                eth_cols.append(i)

        # Detect which interface is anomalous by checking for values > 5000 Mbps
        # (no single GbE port should exceed ~1000 Mbps)
        anomalous_cols = set()
        for ri in range(stable_start, len(rows)):
            r = rows[ri]
            for ci in eth_cols:
                if ci < len(r) and float(r[ci]) > 5000:
                    anomalous_cols.add(ci)

        good_cols = [c for c in eth_cols if c not in anomalous_cols]
        if anomalous_cols:
            anom_names = [header[c] for c in anomalous_cols]
            print(f"    Anomaly detected on: {', '.join(anom_names)} — interpolating from {len(good_cols)} good ports")

        for ri in range(stable_start, len(rows)):
            r = rows[ri]
            if idx_tput < len(r):
                raw_tput = float(r[idx_tput])
                tputs.append(raw_tput)

                # Compute corrected throughput: sum of good ports + average of good ports as interpolation
                good_port_tputs = []
                for ci in good_cols:
                    if ci < len(r):
                        good_port_tputs.append(float(r[ci]))

                if good_port_tputs:
                    avg_good_port = statistics.mean(good_port_tputs)
                    # N good ports + 1 interpolated port
                    corrected = sum(good_port_tputs) + avg_good_port
                    corrected_tputs.append(corrected)
    elif idx_tput is not None:
        for ri in range(stable_start, len(rows)):
            r = rows[ri]
            if idx_tput < len(r):
                tputs.append(float(r[idx_tput]))

    result = {
        "phase": "All MAX Load",
        "group": "6",
        **compute_stats(powers),
    }

    if tputs:
        result["raw_tput_mean"] = statistics.mean(tputs)
    if corrected_tputs:
        result["corrected_tput_mean"] = statistics.mean(corrected_tputs)
        result["corrected_tput_std"] = statistics.stdev(corrected_tputs) if len(corrected_tputs) > 1 else 0
    elif tputs:
        result["corrected_tput_mean"] = statistics.mean(tputs)
        result["corrected_tput_std"] = statistics.stdev(tputs) if len(tputs) > 1 else 0

    return [result]


def process_device(device_name, cfg, run="run_a"):
    """Process all groups for a device, producing a unified summary."""
    files = cfg.get(run)
    if files is None:
        return []

    print(f"\n{'='*60}")
    print(f"  {device_name.upper()} - {'Run A (Default)' if run == 'run_a' else 'Run B (Eco/Power-Saving)'}")
    print(f"{'='*60}")

    all_results = []

    # Group 1+2: Bare minimum + cables + USB
    if "1_and_2" in files:
        g12_path = os.path.join(BASE, files["1_and_2"])
        if os.path.exists(g12_path):
            g12 = process_group_1_and_2(g12_path, device_name, cfg)
            all_results.extend(g12)
            print(f"  Group 1+2: {len(g12)} sub-phases")
        else:
            print(f"  Group 1+2: SKIP (file not found: {files['1_and_2']})")
    else:
        print(f"  Group 1+2: SKIP (not in this run)")

    # Group 3: WiFi radios ON (file "_3" = WiFi for all devices)
    # Note: USB was already captured in Group 1+2 files via custom events
    if "3" in files:
        g3_path = os.path.join(BASE, files["3"])
        if device_name == "alcatel":
            g3 = process_simple_group(g3_path, 3, "WiFi 2.4GHz Radio ON")
        else:
            g3 = process_simple_group(g3_path, 3, "WiFi Radios ON")
        all_results.extend(g3)
        print(f"  Group 3: {len(g3)} sub-phases")

    # Group 4: WiFi clients connected (idle)
    if "4" in files:
        g4_path = os.path.join(BASE, files["4"])
        g4 = process_simple_group(g4_path, 4, "WiFi Clients Connected")
        all_results.extend(g4)
        print(f"  Group 4: {len(g4)} sub-phases")

    # Group 5: WiFi load
    if "5" in files:
        g5_path = os.path.join(BASE, files["5"])
        g5 = process_simple_group(g5_path, 5, "WiFi Load")
        all_results.extend(g5)
        print(f"  Group 5: {len(g5)} sub-phases")

    # Group 6: Full load
    if "6" in files:
        g6_path = os.path.join(BASE, files["6"])
        g6 = process_group_6(g6_path, device_name, cfg)
        all_results.extend(g6)
        print(f"  Group 6: {len(g6)} sub-phases")

    # Print summary table
    print(f"\n  {'Phase':<30s} {'Group':>5s} {'AvgPower':>10s} {'StdDev':>8s} {'N':>4s}")
    print(f"  {'-'*30} {'-'*5} {'-'*10} {'-'*8} {'-'*4}")
    for r in all_results:
        extra = ""
        if "corrected_tput_mean" in r:
            extra = f"  ({r['corrected_tput_mean']:.0f} Mbps)"
        print(f"  {r['phase']:<30s} {r['group']:>5s} {r['mean']:>9.0f}W {r['std']:>7.1f} {r['count']:>4d}{extra}")

    return all_results


def write_summary_csv(path, results, device_name, wifi_notes=None):
    """Write per-device summary CSV."""
    with open(path, "w", newline="") as f:
        f.write("Phase,Group,AvgPowerMW,StdDevPowerMW,MinPowerMW,MaxPowerMW,DataPoints")
        if any("corrected_tput_mean" in r for r in results):
            f.write(",EthThroughputMbps,EthThroughputStdMbps")
        f.write("\n")

        for r in results:
            f.write(f"{r['phase']},{r['group']},{r['mean']:.1f},{r['std']:.1f},"
                    f"{r['min']:.1f},{r['max']:.1f},{r['count']}")
            if any("corrected_tput_mean" in r2 for r2 in results):
                tput = r.get("corrected_tput_mean", "")
                tput_std = r.get("corrected_tput_std", "")
                if tput != "":
                    f.write(f",{tput:.1f},{tput_std:.1f}")
                else:
                    f.write(",,")
            f.write("\n")


def build_comparison(all_device_results):
    """Build a cross-device comparison table with standardized phase names."""
    # Standardize phase names across devices for comparison
    # We want: Bare Min, +Cable 1..4, +USB, +WiFi 2.4, +WiFi 5, +WiFi Clients, WiFi Load, All MAX

    comparison = []
    for device_name, results in all_device_results.items():
        for r in results:
            comparison.append({
                "device": device_name,
                "phase": r["phase"],
                "group": r["group"],
                "avg_power_mw": r["mean"],
                "std_power_mw": r["std"],
                "data_points": r["count"],
            })
    return comparison


def write_comparison_csv(path, comparison):
    """Write cross-device comparison CSV."""
    with open(path, "w", newline="") as f:
        f.write("Device,Phase,Group,AvgPowerMW,StdDevPowerMW,DataPoints\n")
        for c in comparison:
            f.write(f"{c['device']},{c['phase']},{c['group']},"
                    f"{c['avg_power_mw']:.1f},{c['std_power_mw']:.1f},{c['data_points']}\n")


if __name__ == "__main__":
    os.chdir(os.path.dirname(os.path.abspath(__file__)))
    os.makedirs("data", exist_ok=True)
    print("=" * 70)
    print("Preprocessing Test 3 CSVs...")
    print("=" * 70)

    all_run_a = {}
    all_run_b = {}

    for device_name, cfg in DEVICES.items():
        # Run A
        results_a = process_device(device_name, cfg, "run_a")
        if results_a:
            all_run_a[device_name] = results_a
            write_summary_csv(f"data/test3_{device_name}_summary.csv", results_a,
                              device_name, WIFI_NOTES.get(device_name))

        # Run B (eco)
        if cfg.get("run_b"):
            results_b = process_device(device_name, cfg, "run_b")
            if results_b:
                all_run_b[device_name] = results_b
                write_summary_csv(f"data/test3_{device_name}_eco_summary.csv", results_b,
                                  device_name, WIFI_NOTES_ECO.get(device_name))

    # Cross-device comparison
    comparison = build_comparison(all_run_a)
    write_comparison_csv("data/test3_comparison.csv", comparison)

    # Also write eco comparison
    if all_run_b:
        comparison_eco = build_comparison(all_run_b)
        write_comparison_csv("data/test3_comparison_eco.csv", comparison_eco)

    print("\n" + "=" * 70)
    print("Done! Files written to data/")
    print("=" * 70)
