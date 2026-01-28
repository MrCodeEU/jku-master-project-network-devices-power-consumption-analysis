// Enhanced Analysis Page JavaScript with Advanced Metrics and Visualizations

let currentTestData = null;
let selectedTestId = null;
let powerChart = null;
let throughputChart = null;
let scatterChart = null;
let phaseChart = null;
let boxPlotChart = null;

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    initializeEventListeners();
});

function initializeEventListeners() {
    document.getElementById('loadFromDbBtn').addEventListener('click', showDbSection);
    document.getElementById('loadFromCsvBtn').addEventListener('click', showCsvSection);
    document.getElementById('searchTests')?.addEventListener('input', handleSearchTests);
    document.getElementById('loadSelectedTestBtn').addEventListener('click', loadSelectedTest);
    document.getElementById('exportExcelBtn').addEventListener('click', exportToExcel);
    document.getElementById('resetZoomBtn')?.addEventListener('click', resetAllZoom);

    // CSV file input
    const fileInput = document.getElementById('csvFileInput');
    const uploadArea = document.getElementById('uploadArea');

    uploadArea.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', (e) => {
        if (e.target.files.length > 0) {
            loadCsvFile(e.target.files[0]);
        }
    });

    // Drag and drop
    uploadArea.addEventListener('dragover', (e) => {
        e.preventDefault();
        uploadArea.classList.add('dragging');
    });

    uploadArea.addEventListener('dragleave', () => {
        uploadArea.classList.remove('dragging');
    });

    uploadArea.addEventListener('drop', (e) => {
        e.preventDefault();
        uploadArea.classList.remove('dragging');
        if (e.dataTransfer.files.length > 0) {
            loadCsvFile(e.dataTransfer.files[0]);
        }
    });

    // Chart control toggles
    document.getElementById('showPowerStats')?.addEventListener('change', (e) => {
        if (powerChart) {
            updateChartAnnotations();
        }
    });

    document.getElementById('showThroughputStats')?.addEventListener('change', (e) => {
        if (throughputChart) {
            updateChartAnnotations();
        }
    });
}

function showDbSection() {
    document.getElementById('dbTestSection').classList.remove('hidden');
    document.getElementById('csvUploadSection').classList.add('hidden');
    loadTestList();
}

function showCsvSection() {
    document.getElementById('dbTestSection').classList.add('hidden');
    document.getElementById('csvUploadSection').classList.remove('hidden');
}

async function loadTestList() {
    const testList = document.getElementById('testList');
    testList.innerHTML = '<div class="loading">Loading tests</div>';

    try {
        const response = await fetch('/tests');
        if (!response.ok) throw new Error('Failed to load tests');

        const tests = await response.json();

        if (tests.length === 0) {
            testList.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-secondary);">No tests found</div>';
            return;
        }

        testList.innerHTML = '';
        tests.forEach(test => {
            const item = createTestListItem(test);
            testList.appendChild(item);
        });
    } catch (err) {
        console.error('Error loading tests:', err);
        testList.innerHTML = `<div style="padding: 20px; text-align: center; color: var(--danger-color);">Error: ${err.message}</div>`;
    }
}

function createTestListItem(test) {
    const item = document.createElement('div');
    item.className = 'test-item';
    item.dataset.testId = test.id;

    const date = new Date(test.timestamp);
    const dateStr = date.toLocaleString();

    item.innerHTML = `
        <div class="title">${test.test_name}</div>
        <div class="details">
            Device: ${test.device_name} | Date: ${dateStr}
        </div>
    `;

    item.addEventListener('click', () => selectTest(test.id, item));

    return item;
}

function selectTest(testId, element) {
    document.querySelectorAll('.test-item').forEach(item => {
        item.classList.remove('selected');
    });

    element.classList.add('selected');
    selectedTestId = testId;

    document.getElementById('loadSelectedTestBtn').disabled = false;
}

function handleSearchTests(e) {
    const query = e.target.value.toLowerCase();
    document.querySelectorAll('.test-item').forEach(item => {
        const title = item.querySelector('.title').textContent.toLowerCase();
        const details = item.querySelector('.details').textContent.toLowerCase();
        if (title.includes(query) || details.includes(query)) {
            item.style.display = '';
        } else {
            item.style.display = 'none';
        }
    });
}

async function loadSelectedTest() {
    if (!selectedTestId) return;

    try {
        const response = await fetch(`/tests/${selectedTestId}`);
        if (!response.ok) throw new Error('Failed to load test data');

        const testRecord = await response.json();

        const config = JSON.parse(testRecord.config);
        const data = JSON.parse(testRecord.data);

        currentTestData = {
            testName: testRecord.test_name,
            deviceName: testRecord.device_name,
            timestamp: new Date(testRecord.timestamp),
            config: config,
            dataPoints: data
        };

        analyzeAndDisplayTest();
    } catch (err) {
        console.error('Error loading test:', err);
        alert(`Error loading test: ${err.message}`);
    }
}

function loadCsvFile(file) {
    const reader = new FileReader();
    reader.onload = (e) => {
        try {
            parseCsvAndDisplay(e.target.result, file.name);
        } catch (err) {
            console.error('Error parsing CSV:', err);
            alert(`Error parsing CSV: ${err.message}`);
        }
    };
    reader.readAsText(file);
}

function parseCsvAndDisplay(csvText, filename) {
    const lines = csvText.split('\n');
    let dataStart = 0;
    let testName = filename.replace('.csv', '');
    let deviceName = 'Unknown Device';

    // Parse metadata from comments
    for (let i = 0; i < lines.length; i++) {
        const line = lines[i].trim();
        if (line.startsWith('#')) {
            if (line.includes('Test Name:')) {
                testName = line.split('Test Name:')[1]?.trim() || testName;
            }
            if (line.includes('Device:')) {
                deviceName = line.split('Device:')[1]?.trim() || deviceName;
            }
        } else {
            dataStart = i;
            break;
        }
    }

    // Parse CSV headers and data
    const headers = lines[dataStart].split(',').map(h => h.trim());
    const dataPoints = [];

    for (let i = dataStart + 1; i < lines.length; i++) {
        const line = lines[i].trim();
        if (!line) continue;

        const values = line.split(',').map(v => v.trim());
        if (values.length !== headers.length) continue;

        const point = {};
        headers.forEach((header, idx) => {
            const value = values[idx];

            if (header === 'Timestamp') {
                point.timestamp = new Date(value);
            } else if (header === 'ElapsedSeconds') {
                point.elapsed_seconds = parseInt(value);
            } else if (header === 'PowerMW') {
                point.power_mw = parseFloat(value);
            } else if (header === 'ThroughputTotalMbps') {
                point.throughput_mbps = parseFloat(value);
            } else if (header === 'Phase') {
                point.phase = value;
            } else if (header === 'Events') {
                point.events = value ? value.split('|').map(e => ({ message: e.trim() })) : [];
            } else {
                point[header] = value;
            }
        });

        dataPoints.push(point);
    }

    currentTestData = {
        testName: testName,
        deviceName: deviceName,
        timestamp: dataPoints[0]?.timestamp || new Date(),
        config: {},
        dataPoints: dataPoints
    };

    analyzeAndDisplayTest();
}

function analyzeAndDisplayTest() {
    if (!currentTestData || !currentTestData.dataPoints || currentTestData.dataPoints.length === 0) {
        alert('No data to analyze');
        return;
    }

    // Get outlier threshold
    const outlierThreshold = parseFloat(document.getElementById('outlierThreshold')?.value) || 0;

    // Filter outliers if threshold is set
    let filteredDataPoints = currentTestData.dataPoints;
    if (outlierThreshold > 0) {
        const originalCount = filteredDataPoints.length;
        filteredDataPoints = filteredDataPoints.filter(dp => {
            const throughput = dp.throughput_mbps || 0;
            return throughput <= outlierThreshold;
        });
        const removedCount = originalCount - filteredDataPoints.length;
        if (removedCount > 0) {
            console.log(`Filtered ${removedCount} outlier data points (threshold: ${outlierThreshold} Mbps)`);
        }
    }

    // Calculate comprehensive statistics
    const stats = calculateAdvancedStatistics(filteredDataPoints);

    // Display results
    displayTestInfo(currentTestData, filteredDataPoints.length);
    displaySummaryStats(stats);
    displayPhaseCards(stats);
    displayComparisonTable(stats);
    renderAllCharts(filteredDataPoints, stats);

    // Show analysis section
    document.getElementById('analysisSection').classList.remove('hidden');

    // Scroll to results
    document.getElementById('analysisSection').scrollIntoView({ behavior: 'smooth' });
}

function calculateAdvancedStatistics(dataPoints) {
    const stats = {
        totalPoints: dataPoints.length,
        duration: 0,
        avgPower: 0,
        minPower: Infinity,
        maxPower: -Infinity,
        powerStdDev: 0,
        avgThroughput: 0,
        minThroughput: Infinity,
        maxThroughput: -Infinity,
        throughputStdDev: 0,
        avgEfficiency: 0,
        phaseStats: {},
        markerPhases: [],
        phaseChanges: []
    };

    if (dataPoints.length === 0) return stats;

    // Calculate overall stats
    let totalPower = 0;
    let totalThroughput = 0;
    const powerValues = [];
    const throughputValues = [];

    dataPoints.forEach(dp => {
        const power = dp.power_mw || 0;
        const throughput = dp.throughput_mbps || 0;

        totalPower += power;
        totalThroughput += throughput;
        powerValues.push(power);
        throughputValues.push(throughput);

        if (power < stats.minPower) stats.minPower = power;
        if (power > stats.maxPower) stats.maxPower = power;
        if (throughput < stats.minThroughput) stats.minThroughput = throughput;
        if (throughput > stats.maxThroughput) stats.maxThroughput = throughput;
    });

    stats.avgPower = totalPower / dataPoints.length;
    stats.avgThroughput = totalThroughput / dataPoints.length;
    stats.powerStdDev = calculateStdDev(powerValues, stats.avgPower);
    stats.throughputStdDev = calculateStdDev(throughputValues, stats.avgThroughput);
    stats.avgEfficiency = stats.avgPower > 0 ? stats.avgThroughput / (stats.avgPower / 1000) : 0;

    // Calculate duration
    if (dataPoints.length > 0) {
        const firstTime = dataPoints[0].elapsed_seconds || 0;
        const lastTime = dataPoints[dataPoints.length - 1].elapsed_seconds || 0;
        stats.duration = lastTime - firstTime;
    }

    // Identify phase markers
    const markerEvents = [];
    dataPoints.forEach((dp, idx) => {
        if (dp.events && dp.events.length > 0) {
            dp.events.forEach(event => {
                if (event.message && event.message.trim()) {
                    const eventParts = event.message.split('|').map(p => p.trim());

                    eventParts.forEach(part => {
                        if (part.includes('[phase]') || part.includes('[iface_start]')) {
                            markerEvents.push({
                                index: idx,
                                time: dp.elapsed_seconds,
                                message: part,
                                phase: dp.phase,
                                isPhaseMarker: part.includes('[phase]'),
                                isInterfaceStart: part.includes('[iface_start]')
                            });
                        }
                    });
                }
            });
        }
    });

    // Create phases between markers
    if (markerEvents.length > 0) {
        const filteredMarkers = markerEvents.filter((marker, idx) => {
            if (marker.isPhaseMarker && marker.message.includes('Load Test')) {
                const hasInterfaceStartAtSameTime = markerEvents.some((m, mIdx) =>
                    mIdx !== idx && m.time === marker.time && m.isInterfaceStart
                );
                return !hasInterfaceStartAtSameTime;
            }
            return true;
        });

        for (let i = 0; i < filteredMarkers.length; i++) {
            const startEvent = filteredMarkers[i];
            const endEvent = filteredMarkers[i + 1];

            const startIdx = startEvent.index;
            const endIdx = endEvent ? endEvent.index : dataPoints.length - 1;

            let phaseName = startEvent.message;
            phaseName = phaseName.replace(/\[phase\]/gi, '').replace(/\[iface_start\]/gi, '').trim();

            if (phaseName.includes('Interface') && phaseName.includes('started')) {
                const match = phaseName.match(/Interface\s+(.+?)\s+started/);
                if (match) {
                    phaseName = match[1].trim();
                }
            }

            if (!phaseName || (endEvent && endEvent.time - startEvent.time < 1)) {
                continue;
            }

            const phasePoints = dataPoints.slice(startIdx, endIdx + 1);
            const endTime = endEvent ? endEvent.time : dataPoints[dataPoints.length - 1].elapsed_seconds;
            const phaseStats = calculatePhaseStats(phasePoints, phaseName, startEvent.time, endTime);

            if (phaseStats.duration > 0) {
                stats.phaseStats[phaseName] = phaseStats;
                stats.markerPhases.push({
                    name: phaseName,
                    startTime: startEvent.time,
                    endTime: endTime,
                    startIdx: startIdx,
                    endIdx: endIdx,
                    stats: phaseStats
                });
            }
        }
    } else {
        // Fallback to standard phase-based grouping
        const phaseData = {};
        dataPoints.forEach(dp => {
            const phase = dp.phase || 'unknown';
            if (!phaseData[phase]) {
                phaseData[phase] = [];
            }
            phaseData[phase].push(dp);
        });

        Object.keys(phaseData).forEach(phase => {
            const points = phaseData[phase];
            if (points.length === 0) return;

            const startTime = points[0].elapsed_seconds || 0;
            const endTime = points[points.length - 1].elapsed_seconds || 0;
            stats.phaseStats[phase] = calculatePhaseStats(points, phase, startTime, endTime);
        });
    }

    // Calculate phase changes (percentage increases/decreases between phases)
    const phaseKeys = Object.keys(stats.phaseStats);
    for (let i = 1; i < phaseKeys.length; i++) {
        const prevPhase = phaseKeys[i - 1];
        const currentPhase = phaseKeys[i];
        const prevStats = stats.phaseStats[prevPhase];
        const currentStats = stats.phaseStats[currentPhase];

        const powerChange = prevStats.avgPower > 0
            ? ((currentStats.avgPower - prevStats.avgPower) / prevStats.avgPower) * 100
            : 0;

        const throughputChange = prevStats.avgThroughput > 0
            ? ((currentStats.avgThroughput - prevStats.avgThroughput) / prevStats.avgThroughput) * 100
            : 0;

        stats.phaseChanges.push({
            from: prevPhase,
            to: currentPhase,
            powerChange: powerChange,
            throughputChange: throughputChange
        });
    }

    return stats;
}

function calculatePhaseStats(points, phaseName, startTime, endTime) {
    if (points.length === 0) {
        return {
            count: 0,
            duration: 0,
            avgPower: 0,
            minPower: 0,
            maxPower: 0,
            powerStdDev: 0,
            avgThroughput: 0,
            minThroughput: 0,
            maxThroughput: 0,
            throughputStdDev: 0,
            jitter: 0,
            efficiency: 0
        };
    }

    let powerSum = 0;
    let throughputSum = 0;
    const powerValues = [];
    const throughputValues = [];
    let minPower = Infinity;
    let maxPower = -Infinity;
    let minThroughput = Infinity;
    let maxThroughput = -Infinity;

    points.forEach(dp => {
        const power = dp.power_mw || 0;
        const throughput = dp.throughput_mbps || 0;
        powerSum += power;
        throughputSum += throughput;
        powerValues.push(power);
        throughputValues.push(throughput);

        if (power < minPower) minPower = power;
        if (power > maxPower) maxPower = power;
        if (throughput < minThroughput) minThroughput = throughput;
        if (throughput > maxThroughput) maxThroughput = throughput;
    });

    const avgPower = powerSum / points.length;
    const avgThroughput = throughputSum / points.length;

    const powerStdDev = calculateStdDev(powerValues, avgPower);
    const throughputStdDev = calculateStdDev(throughputValues, avgThroughput);

    // Jitter is the standard deviation of throughput
    const jitter = throughputStdDev;

    // Efficiency (Mbps per Watt)
    const efficiency = avgPower > 0 ? avgThroughput / (avgPower / 1000) : 0;

    return {
        count: points.length,
        duration: endTime - startTime,
        avgPower: avgPower,
        minPower: minPower,
        maxPower: maxPower,
        powerStdDev: powerStdDev,
        avgThroughput: avgThroughput,
        minThroughput: minThroughput,
        maxThroughput: maxThroughput,
        throughputStdDev: throughputStdDev,
        jitter: jitter,
        efficiency: efficiency
    };
}

function calculateStdDev(values, mean) {
    if (values.length === 0) return 0;
    const squaredDiffs = values.map(v => Math.pow(v - mean, 2));
    const variance = squaredDiffs.reduce((a, b) => a + b, 0) / values.length;
    return Math.sqrt(variance);
}

function displayTestInfo(testData, filteredCount) {
    const infoDiv = document.getElementById('testInfo');
    const originalCount = testData.dataPoints.length;
    const removedCount = originalCount - (filteredCount || originalCount);

    let dataPointsText = `${filteredCount || originalCount}`;
    if (removedCount > 0) {
        dataPointsText += ` <span class="badge badge-warning">${removedCount} outliers removed</span>`;
    } else {
        dataPointsText += ` <span class="badge badge-success">All data included</span>`;
    }

    infoDiv.innerHTML = `
        <div class="info-item">
            <div class="info-label">Test Name</div>
            <div class="info-value">${testData.testName}</div>
        </div>
        <div class="info-item">
            <div class="info-label">Device</div>
            <div class="info-value">${testData.deviceName}</div>
        </div>
        <div class="info-item">
            <div class="info-label">Timestamp</div>
            <div class="info-value">${testData.timestamp.toLocaleString()}</div>
        </div>
        <div class="info-item">
            <div class="info-label">Data Points</div>
            <div class="info-value">${dataPointsText}</div>
        </div>
    `;
}

function displaySummaryStats(stats) {
    const grid = document.getElementById('statsGrid');

    const powerRange = stats.maxPower - stats.minPower;
    const throughputRange = stats.maxThroughput - stats.minThroughput;

    grid.innerHTML = `
        <div class="stat-card blue">
            <div class="label">Average Power Consumption</div>
            <div class="value">${(stats.avgPower / 1000).toFixed(2)}</div>
            <div class="unit">Watts</div>
            <div class="change">±${(stats.powerStdDev / 1000).toFixed(2)} W</div>
        </div>
        <div class="stat-card green">
            <div class="label">Power Range</div>
            <div class="value">${(powerRange / 1000).toFixed(2)}</div>
            <div class="unit">Watts</div>
            <div class="change">${(stats.minPower / 1000).toFixed(2)} - ${(stats.maxPower / 1000).toFixed(2)} W</div>
        </div>
        <div class="stat-card orange">
            <div class="label">Average Throughput</div>
            <div class="value">${stats.avgThroughput.toFixed(1)}</div>
            <div class="unit">Mbps</div>
            <div class="change">Jitter: ±${stats.throughputStdDev.toFixed(1)} Mbps</div>
        </div>
        <div class="stat-card purple">
            <div class="label">Throughput Range</div>
            <div class="value">${throughputRange.toFixed(1)}</div>
            <div class="unit">Mbps</div>
            <div class="change">${stats.minThroughput.toFixed(1)} - ${stats.maxThroughput.toFixed(1)} Mbps</div>
        </div>
        <div class="stat-card red">
            <div class="label">Average Efficiency</div>
            <div class="value">${stats.avgEfficiency.toFixed(1)}</div>
            <div class="unit">Mbps/W</div>
            <div class="change">Energy Performance Index</div>
        </div>
        <div class="stat-card">
            <div class="label">Test Duration</div>
            <div class="value">${Math.floor(stats.duration)}</div>
            <div class="unit">seconds</div>
            <div class="change">${stats.totalPoints} data points</div>
        </div>
    `;
}

function displayPhaseCards(stats) {
    const container = document.getElementById('phaseCards');
    container.innerHTML = '';

    Object.keys(stats.phaseStats).forEach((phaseName, idx) => {
        const phase = stats.phaseStats[phaseName];

        // Calculate percentage change from previous phase
        let powerChangeText = '';
        let throughputChangeText = '';

        if (idx > 0 && stats.phaseChanges[idx - 1]) {
            const change = stats.phaseChanges[idx - 1];
            const powerChangeSign = change.powerChange >= 0 ? '+' : '';
            const throughputChangeSign = change.throughputChange >= 0 ? '+' : '';
            powerChangeText = `${powerChangeSign}${change.powerChange.toFixed(1)}% from previous`;
            throughputChangeText = `${throughputChangeSign}${change.throughputChange.toFixed(1)}% from previous`;
        }

        const card = document.createElement('div');
        card.className = 'phase-card';
        card.innerHTML = `
            <div class="phase-card-header">${getDisplayPhaseName(phaseName)}</div>
            <div class="phase-metric">
                <span class="phase-metric-label">Duration</span>
                <span class="phase-metric-value">${phase.duration.toFixed(1)}s</span>
            </div>
            <div class="phase-metric">
                <span class="phase-metric-label">Avg Power</span>
                <span class="phase-metric-value">${(phase.avgPower / 1000).toFixed(2)} W</span>
            </div>
            ${powerChangeText ? `<div class="phase-metric">
                <span class="phase-metric-label">Power Change</span>
                <span class="phase-metric-value metric-highlight">${powerChangeText}</span>
            </div>` : ''}
            <div class="phase-metric">
                <span class="phase-metric-label">Power Range</span>
                <span class="phase-metric-value">${(phase.minPower / 1000).toFixed(2)} - ${(phase.maxPower / 1000).toFixed(2)} W</span>
            </div>
            <div class="phase-metric">
                <span class="phase-metric-label">Avg Throughput</span>
                <span class="phase-metric-value">${phase.avgThroughput.toFixed(1)} Mbps</span>
            </div>
            ${throughputChangeText ? `<div class="phase-metric">
                <span class="phase-metric-label">Throughput Change</span>
                <span class="phase-metric-value metric-highlight">${throughputChangeText}</span>
            </div>` : ''}
            <div class="phase-metric">
                <span class="phase-metric-label">Jitter (StdDev)</span>
                <span class="phase-metric-value">${phase.jitter.toFixed(2)} Mbps</span>
            </div>
            <div class="phase-metric">
                <span class="phase-metric-label">Efficiency</span>
                <span class="phase-metric-value">${phase.efficiency.toFixed(1)} Mbps/W</span>
            </div>
            <div class="phase-metric">
                <span class="phase-metric-label">Data Points</span>
                <span class="phase-metric-value">${phase.count}</span>
            </div>
        `;
        container.appendChild(card);
    });
}

function displayComparisonTable(stats) {
    const tbody = document.querySelector('#comparisonTable tbody');
    tbody.innerHTML = '';

    Object.keys(stats.phaseStats).forEach((phaseName, idx) => {
        const phase = stats.phaseStats[phaseName];
        const row = tbody.insertRow();

        let powerChangeText = '-';
        if (idx > 0 && stats.phaseChanges[idx - 1]) {
            const change = stats.phaseChanges[idx - 1];
            const sign = change.powerChange >= 0 ? '+' : '';
            powerChangeText = `${sign}${change.powerChange.toFixed(1)}%`;
        }

        row.innerHTML = `
            <td><strong>${getDisplayPhaseName(phaseName)}</strong></td>
            <td>${phase.duration.toFixed(1)}s</td>
            <td>${(phase.avgPower / 1000).toFixed(2)} W<br><small style="color: var(--text-secondary);">±${(phase.powerStdDev / 1000).toFixed(2)} W</small></td>
            <td>${powerChangeText}</td>
            <td>${phase.avgThroughput.toFixed(1)} Mbps</td>
            <td>${phase.jitter.toFixed(2)} Mbps</td>
            <td>${phase.efficiency.toFixed(1)} Mbps/W</td>
        `;
    });
}

function getDisplayPhaseName(phase) {
    if (phase === 'pre') return 'Pre-Test Baseline';
    if (phase === 'load') return 'Load Test';
    if (phase === 'post') return 'Post-Test Baseline';
    return phase;
}

function renderAllCharts(dataPoints, stats) {
    // Destroy existing charts
    if (powerChart) powerChart.destroy();
    if (throughputChart) throughputChart.destroy();
    if (scatterChart) scatterChart.destroy();
    if (phaseChart) phaseChart.destroy();
    if (boxPlotChart) boxPlotChart.destroy();

    // Prepare data as {x, y} pairs for linear scale
    const powerData = dataPoints.map(dp => ({
        x: dp.elapsed_seconds,
        y: dp.power_mw / 1000
    }));
    const throughputData = dataPoints.map(dp => ({
        x: dp.elapsed_seconds,
        y: dp.throughput_mbps || 0
    }));

    // Create phase annotations
    const { annotations, legendData } = createPhaseAnnotations(dataPoints, stats);

    // Common zoom configuration
    const zoomOptions = {
        zoom: {
            wheel: { enabled: true },
            pinch: { enabled: true },
            mode: 'x'
        },
        pan: {
            enabled: true,
            mode: 'x'
        },
        limits: {
            x: { min: 'original', max: 'original' }
        }
    };

    // Power Chart
    const powerCtx = document.getElementById('powerChart').getContext('2d');
    powerChart = new Chart(powerCtx, {
        type: 'line',
        data: {
            datasets: [{
                label: 'Power (W)',
                data: powerData,
                borderColor: 'rgb(0, 0, 0)',
                backgroundColor: 'rgba(0, 0, 0, 0.05)',
                borderWidth: 2,
                pointRadius: 0,
                fill: true
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                intersect: false,
                mode: 'index'
            },
            plugins: {
                annotation: { annotations: annotations },
                legend: {
                    display: true,
                    labels: {
                        generateLabels: (chart) => {
                            const defaultLabels = Chart.defaults.plugins.legend.labels.generateLabels(chart);
                            return defaultLabels.concat(legendData);
                        }
                    }
                },
                tooltip: {
                    callbacks: {
                        title: (items) => `Time: ${items[0].parsed.x.toFixed(0)}s`,
                        label: (item) => `Power: ${item.parsed.y.toFixed(2)} W`
                    }
                },
                zoom: zoomOptions
            },
            scales: {
                x: {
                    type: 'linear',
                    title: { display: true, text: 'Time (seconds)' }
                },
                y: {
                    title: { display: true, text: 'Power (W)' },
                    beginAtZero: false
                }
            }
        }
    });

    // Throughput Chart
    const throughputCtx = document.getElementById('throughputChart').getContext('2d');
    throughputChart = new Chart(throughputCtx, {
        type: 'line',
        data: {
            datasets: [{
                label: 'Throughput (Mbps)',
                data: throughputData,
                borderColor: 'rgb(0, 0, 0)',
                backgroundColor: 'rgba(0, 0, 0, 0.05)',
                borderWidth: 2,
                pointRadius: 0,
                fill: true
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                intersect: false,
                mode: 'index'
            },
            plugins: {
                annotation: { annotations: annotations },
                legend: {
                    display: true,
                    labels: {
                        generateLabels: (chart) => {
                            const defaultLabels = Chart.defaults.plugins.legend.labels.generateLabels(chart);
                            return defaultLabels.concat(legendData);
                        }
                    }
                },
                tooltip: {
                    callbacks: {
                        title: (items) => `Time: ${items[0].parsed.x.toFixed(0)}s`,
                        label: (item) => `Throughput: ${item.parsed.y.toFixed(1)} Mbps`
                    }
                },
                zoom: zoomOptions
            },
            scales: {
                x: {
                    type: 'linear',
                    title: { display: true, text: 'Time (seconds)' }
                },
                y: {
                    title: { display: true, text: 'Throughput (Mbps)' },
                    beginAtZero: true
                }
            }
        }
    });

    // Scatter Chart (Power vs Throughput) - already uses {x, y} format correctly
    const scatterData = dataPoints.map(dp => ({
        x: dp.throughput_mbps || 0,
        y: dp.power_mw / 1000
    }));

    const scatterCtx = document.getElementById('scatterChart').getContext('2d');
    scatterChart = new Chart(scatterCtx, {
        type: 'scatter',
        data: {
            datasets: [{
                label: 'Power vs Throughput',
                data: scatterData,
                backgroundColor: 'rgba(139, 92, 246, 0.6)',
                borderColor: 'rgb(139, 92, 246)',
                pointRadius: 4
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: true },
                tooltip: {
                    callbacks: {
                        label: (item) => `Throughput: ${item.parsed.x.toFixed(1)} Mbps, Power: ${item.parsed.y.toFixed(2)} W`
                    }
                }
            },
            scales: {
                x: {
                    type: 'linear',
                    title: { display: true, text: 'Throughput (Mbps)' },
                    beginAtZero: true
                },
                y: {
                    type: 'linear',
                    title: { display: true, text: 'Power (W)' },
                    beginAtZero: false
                }
            }
        }
    });

    // Phase Distribution Pie Chart
    const phaseCtx = document.getElementById('phaseChart').getContext('2d');
    const phaseLabels = Object.keys(stats.phaseStats).map(p => getDisplayPhaseName(p));
    const phaseDurations = Object.values(stats.phaseStats).map(p => p.duration);
    const phaseColors = [
        'rgba(59, 130, 246, 0.8)',
        'rgba(16, 185, 129, 0.8)',
        'rgba(245, 158, 11, 0.8)',
        'rgba(139, 92, 246, 0.8)',
        'rgba(239, 68, 68, 0.8)'
    ];

    phaseChart = new Chart(phaseCtx, {
        type: 'doughnut',
        data: {
            labels: phaseLabels,
            datasets: [{
                label: 'Duration (s)',
                data: phaseDurations,
                backgroundColor: phaseColors,
                borderWidth: 2,
                borderColor: '#fff'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    position: 'right'
                },
                tooltip: {
                    callbacks: {
                        label: (item) => `${item.label}: ${item.parsed.toFixed(1)}s (${((item.parsed / stats.duration) * 100).toFixed(1)}%)`
                    }
                }
            }
        }
    });

    // Box Plot Chart (Power distribution per phase)
    renderBoxPlot(stats);
}

function renderBoxPlot(stats) {
    const ctx = document.getElementById('boxPlotChart').getContext('2d');
    const phaseNames = Object.keys(stats.phaseStats).map(p => getDisplayPhaseName(p));
    const phaseData = Object.values(stats.phaseStats);

    // Distinct colors for each phase (lighter for Q1, darker for Q3)
    const boxPlotColors = [
        { q1: 'rgba(59, 130, 246, 0.4)', q3: 'rgba(59, 130, 246, 0.7)' },      // Blue
        { q1: 'rgba(16, 185, 129, 0.4)', q3: 'rgba(16, 185, 129, 0.7)' },      // Green
        { q1: 'rgba(245, 158, 11, 0.4)', q3: 'rgba(245, 158, 11, 0.7)' },      // Orange
        { q1: 'rgba(139, 92, 246, 0.4)', q3: 'rgba(139, 92, 246, 0.7)' },      // Purple
        { q1: 'rgba(239, 68, 68, 0.4)', q3: 'rgba(239, 68, 68, 0.7)' },        // Red
        { q1: 'rgba(236, 72, 153, 0.4)', q3: 'rgba(236, 72, 153, 0.7)' },      // Pink
        { q1: 'rgba(20, 184, 166, 0.4)', q3: 'rgba(20, 184, 166, 0.7)' },      // Teal
        { q1: 'rgba(251, 146, 60, 0.4)', q3: 'rgba(251, 146, 60, 0.7)' },      // Light Orange
        { q1: 'rgba(168, 85, 247, 0.4)', q3: 'rgba(168, 85, 247, 0.7)' },      // Violet
        { q1: 'rgba(34, 197, 94, 0.4)', q3: 'rgba(34, 197, 94, 0.7)' }         // Light Green
    ];

    // Create separate datasets for each phase to get different colors
    const datasets = phaseNames.map((name, idx) => {
        const phase = phaseData[idx];
        const avgPower = phase.avgPower / 1000;
        const minPower = phase.minPower / 1000;
        const maxPower = phase.maxPower / 1000;
        const stdDev = phase.powerStdDev / 1000;

        // Calculate quartiles (approximation based on normal distribution)
        const q1 = Math.max(minPower, avgPower - 0.675 * stdDev);
        const q3 = Math.min(maxPower, avgPower + 0.675 * stdDev);

        const colors = boxPlotColors[idx % boxPlotColors.length];

        return {
            label: name,
            data: [{
                min: minPower,
                q1: q1,
                median: avgPower,
                q3: q3,
                max: maxPower
            }],
            backgroundColor: [colors.q1],
            borderColor: 'rgb(0, 0, 0)',
            borderWidth: 2,
            outlierColor: 'rgb(239, 68, 68)',
            medianColor: 'rgb(0, 0, 0)',
            lowerBackgroundColor: colors.q1,
            upperBackgroundColor: colors.q3,
            itemRadius: 0,
            itemStyle: 'circle',
            itemBackgroundColor: 'rgb(239, 68, 68)',
            itemBorderColor: 'rgb(0, 0, 0)',
            meanRadius: 0
        };
    });

    boxPlotChart = new Chart(ctx, {
        type: 'boxplot',
        data: {
            labels: [''],
            datasets: datasets
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: {
                    display: true,
                    position: 'bottom'
                },
                tooltip: {
                    callbacks: {
                        label: (item) => {
                            const data = item.raw;
                            return [
                                `${item.dataset.label}:`,
                                `Min: ${data.min.toFixed(2)} W`,
                                `Q1: ${data.q1.toFixed(2)} W`,
                                `Median: ${data.median.toFixed(2)} W`,
                                `Q3: ${data.q3.toFixed(2)} W`,
                                `Max: ${data.max.toFixed(2)} W`
                            ];
                        }
                    }
                }
            },
            scales: {
                x: {
                    title: { display: true, text: 'Phases' }
                },
                y: {
                    title: { display: true, text: 'Power (W)' },
                    beginAtZero: false
                }
            }
        }
    });
}

function createPhaseAnnotations(dataPoints, stats) {
    const annotations = {};
    const legendData = [];

    const colorPalette = [
        'rgba(59, 130, 246, 0.2)',     // Blue
        'rgba(16, 185, 129, 0.2)',     // Green
        'rgba(245, 158, 11, 0.2)',     // Orange
        'rgba(139, 92, 246, 0.2)',     // Purple
        'rgba(239, 68, 68, 0.2)',      // Red
        'rgba(236, 72, 153, 0.2)',     // Pink
        'rgba(20, 184, 166, 0.2)',     // Teal
        'rgba(251, 146, 60, 0.2)',     // Light Orange
        'rgba(168, 85, 247, 0.2)',     // Violet
        'rgba(34, 197, 94, 0.2)'       // Light Green
    ];

    console.log('=== Phase Annotation Debug ===');
    console.log('Total data points:', dataPoints.length);
    console.log('Number of phases in stats:', Object.keys(stats.phaseStats).length);
    console.log('Phase names:', Object.keys(stats.phaseStats));

    // Try marker phases first
    if (stats.markerPhases && stats.markerPhases.length > 0) {
        console.log(`Marker phases found: ${stats.markerPhases.length}`);

        let validMarkerCount = 0;
        stats.markerPhases.forEach((phase, idx) => {
            console.log(`Marker phase ${idx}: "${phase.name}" from ${phase.startTime}s to ${phase.endTime}s (duration: ${phase.endTime - phase.startTime}s)`);

            const color = colorPalette[idx % colorPalette.length];
            const solidColor = color.replace('0.2', '0.9');

            if (phase.startTime !== undefined && phase.endTime !== undefined && phase.endTime > phase.startTime) {
                annotations[`marker_phase_${idx}`] = {
                    type: 'box',
                    xMin: phase.startTime,
                    xMax: phase.endTime,
                    backgroundColor: color,
                    borderWidth: 0,
                    drawTime: 'beforeDatasetsDraw'
                };

                legendData.push({
                    text: getDisplayPhaseName(phase.name),
                    fillStyle: solidColor,
                    strokeStyle: solidColor,
                    lineWidth: 0,
                    hidden: false
                });

                validMarkerCount++;
            }
        });

        console.log(`Created ${validMarkerCount} valid marker phase annotations`);

        // If marker phases cover everything, we're done
        if (validMarkerCount === Object.keys(stats.phaseStats).length) {
            console.log('Marker phases match phaseStats count, using marker phases');
            console.log('Final annotations:', annotations);
            return { annotations, legendData };
        }
    }

    // Fallback: scan data points for phase transitions
    console.log('Using data point scan for phase detection');

    // Clear any previous data
    Object.keys(annotations).forEach(key => delete annotations[key]);
    legendData.length = 0;

    // Build phase regions by scanning data points
    const phaseRegions = [];
    let currentPhase = null;
    let regionStart = null;
    let regionStartIdx = null;

    dataPoints.forEach((dp, idx) => {
        const dpPhase = dp.phase || 'unknown';
        const dpTime = dp.elapsed_seconds;

        if (dpPhase !== currentPhase) {
            // End previous region
            if (currentPhase !== null && regionStart !== null) {
                const regionEnd = idx > 0 ? dataPoints[idx - 1].elapsed_seconds : dpTime;
                phaseRegions.push({
                    phase: currentPhase,
                    startTime: regionStart,
                    endTime: regionEnd,
                    startIdx: regionStartIdx,
                    endIdx: idx - 1
                });
                console.log(`Phase region: "${currentPhase}" from ${regionStart}s to ${regionEnd}s (${idx - regionStartIdx} points)`);
            }

            // Start new region
            currentPhase = dpPhase;
            regionStart = dpTime;
            regionStartIdx = idx;
        }

        // Handle last point
        if (idx === dataPoints.length - 1 && currentPhase !== null) {
            phaseRegions.push({
                phase: currentPhase,
                startTime: regionStart,
                endTime: dpTime,
                startIdx: regionStartIdx,
                endIdx: idx
            });
            console.log(`Phase region (last): "${currentPhase}" from ${regionStart}s to ${dpTime}s (${idx - regionStartIdx + 1} points)`);
        }
    });

    console.log(`Found ${phaseRegions.length} phase regions from data`);

    // Create annotations from regions
    const seenPhases = new Set();
    phaseRegions.forEach((region, idx) => {
        const color = colorPalette[idx % colorPalette.length];
        const solidColor = color.replace('0.2', '0.9');

        annotations[`data_phase_${idx}`] = {
            type: 'box',
            xMin: region.startTime,
            xMax: region.endTime,
            backgroundColor: color,
            borderWidth: 0,
            drawTime: 'beforeDatasetsDraw'
        };

        // Add to legend only once per unique phase
        if (!seenPhases.has(region.phase)) {
            seenPhases.add(region.phase);
            legendData.push({
                text: getDisplayPhaseName(region.phase),
                fillStyle: solidColor,
                strokeStyle: solidColor,
                lineWidth: 0,
                hidden: false
            });
        }
    });

    console.log(`Created ${Object.keys(annotations).length} annotations from data point scan`);
    console.log('Annotations:', annotations);
    console.log('=== End Phase Annotation Debug ===');

    return { annotations, legendData };
}

function updateChartAnnotations() {
    if (powerChart) powerChart.update();
    if (throughputChart) throughputChart.update();
}

function resetAllZoom() {
    if (powerChart) powerChart.resetZoom();
    if (throughputChart) throughputChart.resetZoom();
}

async function exportToExcel() {
    if (!currentTestData || !currentTestData.dataPoints) {
        alert('No data to export');
        return;
    }

    try {
        const workbook = new ExcelJS.Workbook();

        // Summary sheet
        const summarySheet = workbook.addWorksheet('Summary');
        summarySheet.addRow(['Test Analysis Summary']);
        summarySheet.addRow(['Test Name:', currentTestData.testName]);
        summarySheet.addRow(['Device:', currentTestData.deviceName]);
        summarySheet.addRow(['Timestamp:', currentTestData.timestamp.toLocaleString()]);
        summarySheet.addRow([]);

        const stats = calculateAdvancedStatistics(currentTestData.dataPoints);
        summarySheet.addRow(['Overall Statistics']);
        summarySheet.addRow(['Average Power (W):', (stats.avgPower / 1000).toFixed(2)]);
        summarySheet.addRow(['Power Range (W):', `${(stats.minPower / 1000).toFixed(2)} - ${(stats.maxPower / 1000).toFixed(2)}`]);
        summarySheet.addRow(['Average Throughput (Mbps):', stats.avgThroughput.toFixed(1)]);
        summarySheet.addRow(['Throughput Range (Mbps):', `${stats.minThroughput.toFixed(1)} - ${stats.maxThroughput.toFixed(1)}`]);
        summarySheet.addRow(['Average Efficiency (Mbps/W):', stats.avgEfficiency.toFixed(2)]);
        summarySheet.addRow([]);

        summarySheet.addRow(['Phase Statistics']);
        summarySheet.addRow(['Phase', 'Duration (s)', 'Avg Power (W)', 'Power StdDev (W)', 'Avg Throughput (Mbps)', 'Jitter (Mbps)', 'Efficiency (Mbps/W)']);

        Object.keys(stats.phaseStats).forEach(phase => {
            const ps = stats.phaseStats[phase];
            summarySheet.addRow([
                getDisplayPhaseName(phase),
                ps.duration.toFixed(0),
                (ps.avgPower / 1000).toFixed(2),
                (ps.powerStdDev / 1000).toFixed(2),
                ps.avgThroughput.toFixed(1),
                ps.jitter.toFixed(2),
                ps.efficiency.toFixed(1)
            ]);
        });

        // Raw data sheet
        const dataSheet = workbook.addWorksheet('Raw Data');
        dataSheet.addRow(['Timestamp', 'Elapsed Seconds', 'Power (mW)', 'Throughput (Mbps)', 'Phase', 'Events']);

        currentTestData.dataPoints.forEach(dp => {
            const events = dp.events ? dp.events.map(e => e.message || e).join(' | ') : '';
            dataSheet.addRow([
                dp.timestamp ? dp.timestamp.toISOString() : '',
                dp.elapsed_seconds || 0,
                dp.power_mw || 0,
                dp.throughput_mbps || 0,
                dp.phase || '',
                events
            ]);
        });

        // Style headers
        [summarySheet, dataSheet].forEach(sheet => {
            sheet.getRow(1).font = { bold: true, size: 14 };
            if (sheet === dataSheet) {
                sheet.getRow(1).fill = {
                    type: 'pattern',
                    pattern: 'solid',
                    fgColor: { argb: 'FFD3D3D3' }
                };
            }
        });

        // Generate and download
        const buffer = await workbook.xlsx.writeBuffer();
        const blob = new Blob([buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
        const url = URL.createObjectURL(blob);

        const a = document.createElement('a');
        a.href = url;
        a.download = `${currentTestData.testName.replace(/[^a-z0-9]/gi, '_')}_analysis.xlsx`;
        a.click();

        URL.revokeObjectURL(url);
        alert('Excel file exported successfully!');
    } catch (err) {
        console.error('Error exporting to Excel:', err);
        alert(`Error exporting to Excel: ${err.message}`);
    }
}
