# pgfplots Chart Templates

These are the LaTeX/pgfplots code templates used in the report.
They can be integrated into the webapp's pgfplots export feature to generate
standalone `.tex` files or compiled to PDF/PNG for faster report compilation.

All templates expect CSV data files in the `data/` directory with these schemas:

- `{device}_data.csv`: `ElapsedSeconds,PowerMW,ThroughputTotalMbps,Phase`
- `{device}_stats.csv`: `Phase,MeanPowerMW,StdPowerMW,MeanThroughputMbps,StdThroughputMbps`
- `{device}_phases.csv`: `Phase,StartSeconds,EndSeconds`
- `{device}_interfaces.csv`: `ElapsedSeconds,InterfaceName,ThroughputMbps`

---

## 1. Power Over Time (Line Chart)

Shows power consumption over the full test duration with vertical phase boundary lines.

```latex
\begin{figure}[H]
    \centering
    \begin{tikzpicture}
        \begin{axis}[
            width=\textwidth,
            height=7cm,
            xlabel={Elapsed Time (s)},
            ylabel={Power (mW)},
            xmin=0, xmax={{MAX_SECONDS}},
            ymin={{POWER_MIN}}, ymax={{POWER_MAX}},
            grid=major,
            grid style={gray!30},
            legend pos=north west,
            legend style={font=\small},
            title={{DEVICE_NAME} --- Power Consumption Over Time},
            % Phase boundary lines — replace with actual phase start times
            extra x ticks={ {{PHASE_BOUNDARIES}} },
            extra x tick labels={},
            extra x tick style={grid=major, grid style={dashed, red!50, line width=0.8pt}},
        ]
            \addplot[blue, very thin, each nth point=1]
                table[col sep=comma, x=ElapsedSeconds, y=PowerMW]
                {data/{{DEVICE_KEY}}_data.csv};
            \addlegendentry{Power}

            % Idle baseline reference line
            \draw[dashed, gray, thin]
                (axis cs:0,{{IDLE_POWER}}) -- (axis cs:{{MAX_SECONDS}},{{IDLE_POWER}});

            % Phase labels (positioned at midpoint of each phase)
            {{PHASE_LABELS}}
        \end{axis}
    \end{tikzpicture}
    \caption{ {{DEVICE_NAME}}: Power consumption over time. }
    \label{fig:{{DEVICE_KEY}}_power}
\end{figure}
```

### Template Variables
| Variable | Description | Example |
|---|---|---|
| `{{MAX_SECONDS}}` | Total test duration in seconds | `6600` |
| `{{POWER_MIN}}` | Y-axis minimum (leave margin below idle) | `4200` |
| `{{POWER_MAX}}` | Y-axis maximum (leave margin above max) | `5800` |
| `{{PHASE_BOUNDARIES}}` | Comma-separated phase start times | `600, 1800, 3000, 4200, 5400` |
| `{{DEVICE_NAME}}` | Full device name | `Fritzbox 7530` |
| `{{DEVICE_KEY}}` | File name key | `fritzbox` |
| `{{IDLE_POWER}}` | Average idle power in mW | `4432` |
| `{{PHASE_LABELS}}` | TikZ node commands for phase labels | see below |

### Phase Label Template
```latex
\node[font=\tiny, rotate=90, anchor=south]
    at (axis cs:{{MIDPOINT}},{{LABEL_Y}}) { {{PHASE_NAME}} };
```

---

## 2. Throughput Over Time (Line Chart)

```latex
\begin{figure}[H]
    \centering
    \begin{tikzpicture}
        \begin{axis}[
            width=\textwidth,
            height=7cm,
            xlabel={Elapsed Time (s)},
            ylabel={Throughput (Mbps)},
            xmin=0, xmax={{MAX_SECONDS}},
            ymin=0, ymax={{THROUGHPUT_MAX}},
            grid=major,
            grid style={gray!30},
            legend pos=north west,
            legend style={font=\small},
            title={{DEVICE_NAME} --- Aggregate Throughput Over Time},
            extra x ticks={ {{PHASE_BOUNDARIES}} },
            extra x tick labels={},
            extra x tick style={grid=major, grid style={dashed, red!50, line width=0.8pt}},
        ]
            \addplot[teal, very thin, each nth point=1]
                table[col sep=comma, x=ElapsedSeconds, y=ThroughputTotalMbps]
                {data/{{DEVICE_KEY}}_data.csv};
            \addlegendentry{Throughput}
        \end{axis}
    \end{tikzpicture}
    \caption{ {{DEVICE_NAME}}: Aggregate throughput over time. }
    \label{fig:{{DEVICE_KEY}}_throughput}
\end{figure>
```

---

## 3. Cross-Device Grouped Bar Chart (Power by Phase)

Compares power consumption across all devices for each load phase.

```latex
\begin{figure}[H]
    \centering
    \begin{tikzpicture}
        \begin{axis}[
            ybar,
            width=\textwidth,
            height=8cm,
            bar width=10pt,
            xlabel={Load Phase},
            ylabel={Average Power (mW)},
            ymin=0, ymax={{Y_MAX}},
            symbolic x coords={Idle, 1 Port, 2 Ports, 3 Ports, 4 Ports},
            xtick=data,
            legend pos=north west,
            legend style={font=\small},
            grid=major,
            grid style={gray!20},
            title={Power Consumption by Port Count --- All Devices},
        ]
            % One \addplot per device
            {{#DEVICES}}
            \addplot[fill={{COLOR}}] coordinates {
                (Idle,{{IDLE}})
                (1 Port,{{P1}})
                (2 Ports,{{P2}})
                (3 Ports,{{P3}})
                (4 Ports,{{P4}})
            };
            {{/DEVICES}}
            \legend{ {{DEVICE_NAMES}} }
        \end{axis}
    \end{tikzpicture}
    \caption{Grouped bar chart comparing average power by port count.}
    \label{fig:power_bar_comparison}
\end{figure}
```

### Color Palette
| Device | Fill Color |
|---|---|
| Device 1 | `blue!60` |
| Device 2 | `red!60` |
| Device 3 | `green!60` |
| Device 4 | `orange!60` |

---

## 4. Horizontal Bar Chart (Idle Power Ranking)

```latex
\begin{figure}[H]
    \centering
    \begin{tikzpicture}
        \begin{axis}[
            xbar,
            width=0.85\textwidth,
            height=5cm,
            xlabel={Idle Power (mW)},
            symbolic y coords={ {{DEVICE_NAMES_SORTED_ASC}} },
            ytick=data,
            nodes near coords,
            nodes near coords style={font=\small},
            xmin=0, xmax={{X_MAX}},
            bar width=14pt,
            title={Idle Power Consumption Ranking},
        ]
            \addplot[fill=blue!50] coordinates {
                {{#DEVICES_SORTED}}
                ({{IDLE_POWER}},{{DEVICE_NAME}})
                {{/DEVICES_SORTED}}
            };
        \end{axis}
    \end{tikzpicture}
    \caption{Idle power consumption ranking across all tested devices.}
    \label{fig:idle_power_ranking}
\end{figure}
```

---

## 5. Summary Statistics Table

```latex
\begin{table}[H]
    \centering
    \caption{Summary of key metrics across all tested devices.}
    \label{tab:results-summary}
    \small
    \begin{tabular}{l r r r r}
        \toprule
        \textbf{Metric} & {{DEVICE_HEADERS}} \\
        \midrule
        Idle Power (mW)         & {{IDLE_VALUES}} \\
        Max Load Power (mW)     & {{MAX_POWER_VALUES}} \\
        Power Increase (mW)     & {{DELTA_VALUES}} \\
        Power Increase (\%)     & {{DELTA_PCT_VALUES}} \\
        Max Throughput (Mbps)   & {{MAX_THROUGHPUT_VALUES}} \\
        Peak Efficiency (Mbps/W)& {{EFFICIENCY_VALUES}} \\
        \bottomrule
    \end{tabular}
\end{table}
```

---

## 6. Per-Phase Statistics Table

```latex
\begin{table}[H]
    \centering
    \caption{Average power consumption (mW) by load phase for each device.}
    \label{tab:power-per-phase}
    \small
    \begin{tabular}{l r r r r r r r r}
        \toprule
        \textbf{Device} & \textbf{Idle} & \textbf{1\,Port} & $\Delta$
            & \textbf{2\,Ports} & $\Delta$ & \textbf{3\,Ports}
            & \textbf{4\,Ports} & $\Delta_{\text{max}}$ \\
        \midrule
        {{#DEVICES}}
        {{NAME}} & {{IDLE}} & {{P1}} & +{{D1}} & {{P2}} & +{{D2}}
            & {{P3}} & {{P4}} & +{{DMAX}} ({{DMAX_PCT}}\%) \\
        {{/DEVICES}}
        \bottomrule
    \end{tabular}
\end{table>
```

---

## Required LaTeX Preamble

These packages must be loaded in the document preamble for the above templates to work:

```latex
\usepackage{pgfplots}
\pgfplotsset{compat=1.18}
\usepgfplotslibrary{statistics}
\usepackage{pgfplotstable}
\usepackage{booktabs}
\usepackage{float}   % for [H] placement
\usetikzlibrary{patterns}
```

---

## Webapp Integration Notes

To integrate these templates into the webapp's pgfplots export:

1. **Data export** is already implemented in `analysis.js` → `exportForPgfplots()`.
2. **Template rendering**: Replace `{{VARIABLE}}` placeholders with actual values from the test data.
3. **Phase detection**: The webapp already detects phases via `analyzePhases()` — reuse that logic.
4. **Stats computation**: The webapp's `computePhaseStats()` function provides all needed statistics.
5. **Output options**:
   - **Option A**: Generate a standalone `.tex` file that can be compiled with `lualatex` to produce a PDF
   - **Option B**: Use `tex2svg` or similar to convert to SVG/PNG for direct inclusion via `\includegraphics`
   - **Option C**: Export just the CSV data files (current implementation) and reference them from a manually-written `.tex`

### Standalone Template Wrapper

For generating a compilable standalone document:

```latex
\documentclass[border=5pt]{standalone}
\usepackage{pgfplots}
\pgfplotsset{compat=1.18}
\usepgfplotslibrary{statistics}
\usepackage{pgfplotstable}
\usepackage{booktabs}

\begin{document}
    % Insert chart code here
\end{document}
```

Compile with: `lualatex --shell-escape standalone-chart.tex`
