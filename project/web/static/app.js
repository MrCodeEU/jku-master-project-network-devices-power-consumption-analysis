document.addEventListener('DOMContentLoaded', () => {
    console.log("App initializing...");
    
    const powerCtx = document.getElementById('powerChart').getContext('2d');
    const throughputCtx = document.getElementById('throughputChart').getContext('2d');
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');
    const downloadBtn = document.getElementById('downloadBtn');
    const testFritzBtn = document.getElementById('testFritzBtn');
    const testTargetBtn = document.getElementById('testTargetBtn');
    const statusDiv = document.getElementById('status');
    const form = document.getElementById('testForm');
    const loadEnabledCheckbox = document.getElementById('load_enabled');
    const loadConfigDiv = document.getElementById('load_config');
    const connectionStatusDiv = document.getElementById('connectionStatus');
    const fritzStatusDiv = document.getElementById('fritzStatus');
    const dutStatusDiv = document.getElementById('dutStatus');
    const throughputValueDiv = document.getElementById('throughputValue');
    const throughputPercentDiv = document.getElementById('throughputPercent');

    if (!testFritzBtn || !testTargetBtn) {
        console.error("Buttons not found!");
        return;
    }

    function updateLoadConfigVisibility() {
        if (loadEnabledCheckbox && loadConfigDiv) {
            loadConfigDiv.style.display = loadEnabledCheckbox.checked ? 'block' : 'none';
        }
    }

    // Toggle load config visibility
    if (loadEnabledCheckbox) {
        loadEnabledCheckbox.addEventListener('change', updateLoadConfigVisibility);
        // Initial check
        updateLoadConfigVisibility();
    }

    // Helper to wait
    const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));

    if (testFritzBtn) {
        testFritzBtn.addEventListener('click', async () => {
            console.log("Test Fritzbox clicked");
            connectionStatusDiv.style.display = 'block';
            fritzStatusDiv.innerHTML = '<span class="status-icon">...</span> Fritzbox API: Checking...';
            testFritzBtn.disabled = true;
            testFritzBtn.textContent = 'Checking...';

            try {
                const [response] = await Promise.all([
                    fetch('/test-fritzbox', { method: 'POST' }),
                    wait(500) // Minimum 500ms delay for visual feedback
                ]);
                
                const result = await response.json();
                console.log("Fritzbox result:", result);
                
                if (result.ok) {
                    fritzStatusDiv.innerHTML = '<span class="status-icon status-ok">✓</span> Fritzbox API: Connected';
                } else {
                    fritzStatusDiv.innerHTML = `<span class="status-icon status-error">✗</span> Fritzbox API: Error (${result.error})`;
                }
            } catch (error) {
                console.error('Error testing Fritzbox:', error);
                fritzStatusDiv.innerHTML = '<span class="status-icon status-error">✗</span> Error testing Fritzbox';
            } finally {
                testFritzBtn.disabled = false;
                testFritzBtn.textContent = 'Test Fritzbox';
            }
        });
    }

    if (testTargetBtn) {
        testTargetBtn.addEventListener('click', async () => {
            console.log("Test Target clicked");
            const formData = new FormData(form);
            connectionStatusDiv.style.display = 'block';
            dutStatusDiv.innerHTML = '<span class="status-icon">...</span> Device Under Test: Checking...';
            testTargetBtn.disabled = true;
            testTargetBtn.textContent = 'Checking...';

            try {
                const [response] = await Promise.all([
                    fetch('/test-target', {
                        method: 'POST',
                        body: formData
                    }),
                    wait(500) // Minimum 500ms delay for visual feedback
                ]);

                const result = await response.json();
                console.log("Target result:", result);
                
                if (result.ok) {
                    dutStatusDiv.innerHTML = '<span class="status-icon status-ok">✓</span> Device Under Test: Reachable';
                } else {
                    dutStatusDiv.innerHTML = `<span class="status-icon status-error">✗</span> Device Under Test: Unreachable (${result.error})`;
                }
            } catch (error) {
                console.error('Error testing Target:', error);
                dutStatusDiv.innerHTML = '<span class="status-icon status-error">✗</span> Error testing Target';
            } finally {
                testTargetBtn.disabled = false;
                testTargetBtn.textContent = 'Test Target';
            }
        });
    }

    // Initialize Power Chart
    const powerChart = new Chart(powerCtx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [{
                label: 'Power Consumption (mW)',
                data: [],
                borderColor: 'rgb(75, 192, 192)',
                tension: 0.1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    type: 'linear',
                    position: 'bottom',
                    title: { display: true, text: 'Time (s)' }
                },
                y: {
                    beginAtZero: true,
                    title: { display: true, text: 'Power (mW)' }
                }
            },
            animation: {
                duration: 0 // Disable animation for better performance with real-time data
            }
        }
    });

    // Initialize Throughput Chart
    const throughputChart = new Chart(throughputCtx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [{
                label: 'Throughput (Mbps)',
                data: [],
                borderColor: 'rgb(255, 99, 132)',
                backgroundColor: 'rgba(255, 99, 132, 0.1)',
                fill: true,
                tension: 0.1
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            scales: {
                x: {
                    type: 'linear',
                    position: 'bottom',
                    title: { display: true, text: 'Time (s)' }
                },
                y: {
                    beginAtZero: true,
                    title: { display: true, text: 'Throughput (Mbps)' },
                    max: 1100 // Slightly above 1 Gbps for visualization
                }
            },
            animation: {
                duration: 0
            }
        }
    });

    let eventSource = null;
    let startTime = null;
    let collectedData = [];
    let currentPhase = '';

    // Phase colors for charts
    const phaseColors = {
        'pre': 'rgba(255, 193, 7, 0.3)',   // Yellow for pre-test
        'load': 'rgba(75, 192, 192, 0.3)',  // Teal for load
        'post': 'rgba(108, 117, 125, 0.3)'  // Gray for post-test
    };

    // Connect to SSE
    function connectSSE() {
        if (eventSource) {
            eventSource.close();
        }

        eventSource = new EventSource('/events');

        eventSource.onmessage = function(event) {
            const data = JSON.parse(event.data);
            
            if (!startTime) {
                startTime = new Date(data.timestamp).getTime();
            }

            const currentTime = new Date(data.timestamp).getTime();
            const elapsedSeconds = (currentTime - startTime) / 1000;

            // Update phase status
            const phase = data.phase || 'load';
            if (phase !== currentPhase) {
                currentPhase = phase;
                const phaseNames = { 'pre': 'Pre-Test Baseline', 'load': 'Load Test', 'post': 'Post-Test Baseline' };
                statusDiv.textContent = `Status: Running - ${phaseNames[phase] || phase}`;
            }

            // Update Power Chart
            powerChart.data.labels.push(elapsedSeconds);
            powerChart.data.datasets[0].data.push(data.power_mw);
            powerChart.update();

            // Update Throughput Chart
            const throughputMbps = data.throughput_mbps || 0;
            throughputChart.data.labels.push(elapsedSeconds);
            throughputChart.data.datasets[0].data.push(throughputMbps);
            throughputChart.update();

            // Update throughput display
            const targetThroughput = 1000; // 1 Gbps = 1000 Mbps
            const percentage = (throughputMbps / targetThroughput * 100).toFixed(1);
            throughputValueDiv.textContent = throughputMbps.toFixed(1);
            throughputPercentDiv.textContent = percentage;
            
            // Store for CSV export with phase info
            collectedData.push({
                timestamp: data.timestamp,
                elapsed_seconds: elapsedSeconds,
                power_mw: data.power_mw,
                throughput_mbps: throughputMbps,
                phase: phase
            });
        };

        eventSource.addEventListener('done', function(e) {
            statusDiv.textContent = 'Status: Test Finished';
            startBtn.disabled = false;
            stopBtn.disabled = true;
            downloadBtn.disabled = false;
            eventSource.close();
            eventSource = null;
        });

        eventSource.onerror = function(err) {
            console.error("EventSource failed:", err);
            // Don't close immediately, it might reconnect
        };
    }

    // Start Test
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const formData = new FormData(form);
        
        try {
            const response = await fetch('/start', {
                method: 'POST',
                body: formData
            });

            if (response.ok) {
                statusDiv.textContent = 'Status: Running...';
                startBtn.disabled = true;
                stopBtn.disabled = false;
                downloadBtn.disabled = true;
                
                // Reset charts and data
                powerChart.data.labels = [];
                powerChart.data.datasets[0].data = [];
                powerChart.update();
                
                throughputChart.data.labels = [];
                throughputChart.data.datasets[0].data = [];
                throughputChart.update();
                
                throughputValueDiv.textContent = '0.0';
                throughputPercentDiv.textContent = '0.0';
                
                collectedData = [];
                startTime = null;

                connectSSE();
            } else {
                const text = await response.text();
                alert('Error starting test: ' + text);
            }
        } catch (err) {
            console.error(err);
            alert('Error starting test');
        }
    });

    // Stop Test
    stopBtn.addEventListener('click', async () => {
        try {
            await fetch('/stop', { method: 'POST' });
            statusDiv.textContent = 'Status: Stopped';
            startBtn.disabled = false;
            stopBtn.disabled = true;
            downloadBtn.disabled = false;
            if (eventSource) {
                eventSource.close();
                eventSource = null;
            }
        } catch (err) {
            console.error(err);
        }
    });

    // Download CSV
    downloadBtn.addEventListener('click', () => {
        if (collectedData.length === 0) {
            alert("No data to export");
            return;
        }

        const csvContent = "data:text/csv;charset=utf-8," 
            + "Timestamp,ElapsedSeconds,PowerMW,ThroughputMbps,Phase\n"
            + collectedData.map(e => `${e.timestamp},${e.elapsed_seconds},${e.power_mw},${e.throughput_mbps},${e.phase}`).join("\n");

        const encodedUri = encodeURI(csvContent);
        const link = document.createElement("a");
        link.setAttribute("href", encodedUri);
        link.setAttribute("download", "power_consumption.csv");
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
    });
});
