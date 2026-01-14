document.addEventListener('DOMContentLoaded', () => {
    console.log("App initializing...");

    // LocalStorage key for config persistence
    const CONFIG_STORAGE_KEY = 'PowerTestConfig';

    // IndexedDB for test history storage
    const DB_NAME = 'PowerTestDB';
    const DB_VERSION = 1;
    const STORE_NAME = 'tests';
    let db = null;

    function openDatabase() {
        return new Promise((resolve, reject) => {
            const request = indexedDB.open(DB_NAME, DB_VERSION);
            
            request.onerror = () => reject(request.error);
            request.onsuccess = () => {
                db = request.result;
                resolve(db);
            };
            
            request.onupgradeneeded = (event) => {
                const database = event.target.result;
                if (!database.objectStoreNames.contains(STORE_NAME)) {
                    const store = database.createObjectStore(STORE_NAME, { keyPath: 'id', autoIncrement: true });
                    store.createIndex('timestamp', 'timestamp', { unique: false });
                }
            };
        });
    }

    async function saveTest(testData) {
        if (!db) await openDatabase();
        return new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readwrite');
            const store = tx.objectStore(STORE_NAME);
            const request = store.add(testData);
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
    }

    async function getAllTests() {
        if (!db) await openDatabase();
        return new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readonly');
            const store = tx.objectStore(STORE_NAME);
            const request = store.getAll();
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
    }

    async function deleteTest(id) {
        if (!db) await openDatabase();
        return new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readwrite');
            const store = tx.objectStore(STORE_NAME);
            const request = store.delete(id);
            request.onsuccess = () => resolve();
            request.onerror = () => reject(request.error);
        });
    }

    async function getTest(id) {
        if (!db) await openDatabase();
        return new Promise((resolve, reject) => {
            const tx = db.transaction(STORE_NAME, 'readonly');
            const store = tx.objectStore(STORE_NAME);
            const request = store.get(id);
            request.onsuccess = () => resolve(request.result);
            request.onerror = () => reject(request.error);
        });
    }

    // Initialize database
    openDatabase().catch(err => console.error('Failed to open database:', err));
    
    const powerCtx = document.getElementById('powerChart').getContext('2d');
    const throughputCtx = document.getElementById('throughputChart').getContext('2d');
    const previewCtx = document.getElementById('previewChart').getContext('2d');
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');
    const downloadBtn = document.getElementById('downloadBtn');
    const testFritzBtn = document.getElementById('testFritzBtn');
    const testTargetBtn = document.getElementById('testTargetBtn');
    const togglePreviewBtn = document.getElementById('togglePreviewBtn');
    const previewChartContainer = document.getElementById('previewChartContainer');
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

    const interfaceListDiv = document.getElementById('interfaceList');
    const refreshInterfacesBtn = document.getElementById('refreshInterfacesBtn');

    // Fetch and display network interfaces with per-interface config
    async function loadInterfaces() {
        try {
            interfaceListDiv.innerHTML = '<em>Loading interfaces...</em>';
            const response = await fetch('/interfaces');
            const interfaces = await response.json();
            
            if (interfaces.length === 0) {
                interfaceListDiv.innerHTML = '<em>No network interfaces found</em>';
                return;
            }

            interfaceListDiv.innerHTML = interfaces.map((iface, idx) => `
                <div class="interface-config-card ${idx === 0 ? 'enabled' : ''}" data-iface="${iface.name}">
                    <div class="interface-header">
                        <input type="checkbox" 
                               id="iface_${idx}" 
                               name="interfaces" 
                               value="${iface.name}" 
                               ${idx === 0 ? 'checked' : ''}
                               onchange="this.closest('.interface-config-card').classList.toggle('enabled', this.checked)">
                        <span class="name">${iface.name}</span>
                        <span class="ip">${iface.addresses.join(', ')}</span>
                    </div>
                    <div class="interface-settings">
                        <div class="setting-row">
                            <div class="setting-group">
                                <label>Workers</label>
                                <input type="number" name="workers_${iface.name}" value="10" min="1" max="64" 
                                       title="Worker threads. FEWER workers = better rate control. Recommended: 8-12 for 1 Gbps, 4-6 for lower rates">
                            </div>
                            <div class="setting-group">
                                <label>Target Mbps</label>
                                <input type="number" name="throughput_${iface.name}" value="0" min="0" step="10"
                                       placeholder="0 = max"
                                       title="Target throughput in Mbps (0 = unlimited/maximum speed)">
                            </div>
                            <div class="setting-group">
                                <label>Ramp Steps</label>
                                <input type="number" name="ramp_${iface.name}" value="0" min="0" max="20"
                                       placeholder="0 = none"
                                       title="Number of ramp-up steps (e.g., 4 steps = 25%, 50%, 75%, 100%)">
                            </div>
                        </div>
                        <div class="setting-row">
                            <div class="setting-group">
                                <label>Pre-Delay</label>
                                <input type="text" name="pretime_${iface.name}" value="0s"
                                       placeholder="e.g. 10s"
                                       title="Extra delay before this interface starts (added to global pre-test time). Use format like '10s', '1m'">
                            </div>
                            <div class="setting-group">
                                <label>Ramp Duration</label>
                                <input type="text" name="rampduration_${iface.name}" value="0s"
                                       placeholder="e.g. 30s"
                                       title="How long the ramping takes. 0 = automatic (default based on steps). Use format like '30s', '1m'">
                            </div>
                            <div class="setting-group">
                                <!-- Spacer for alignment -->
                            </div>
                        </div>
                    </div>
                </div>
            `).join('');
            
            // Apply saved interface configs after rendering
            setTimeout(applyInterfaceConfigs, 50);
        } catch (error) {
            console.error('Error loading interfaces:', error);
            interfaceListDiv.innerHTML = '<em>Error loading interfaces</em>';
        }
    }

    // Load interfaces on page load
    loadInterfaces();

    // Refresh button
    if (refreshInterfacesBtn) {
        refreshInterfacesBtn.addEventListener('click', loadInterfaces);
    }

    // ============ Config Persistence ============
    function saveConfigToStorage() {
        try {
            const config = {
                duration: document.getElementById('duration')?.value,
                pollInterval: document.getElementById('poll_interval')?.value,
                preTestTime: document.getElementById('pre_test_time')?.value,
                postTestTime: document.getElementById('post_test_time')?.value,
                powerYMin: document.getElementById('power_y_min')?.value,
                loadEnabled: document.getElementById('load_enabled')?.checked,
                targetIP: document.getElementById('target_ip')?.value,
                targetPort: document.getElementById('target_port')?.value,
                protocol: document.getElementById('protocol')?.value,
                packetSize: document.getElementById('packet_size')?.value,
                // Store interface configs by name
                interfaceConfigs: {}
            };
            
            document.querySelectorAll('.interface-config-card').forEach(card => {
                const ifaceName = card.dataset.iface;
                if (ifaceName) {
                    config.interfaceConfigs[ifaceName] = {
                        enabled: card.classList.contains('enabled'),
                        workers: card.querySelector(`input[name="workers_${ifaceName}"]`)?.value,
                        throughput: card.querySelector(`input[name="throughput_${ifaceName}"]`)?.value,
                        rampSteps: card.querySelector(`input[name="ramp_${ifaceName}"]`)?.value,
                        preTime: card.querySelector(`input[name="pretime_${ifaceName}"]`)?.value,
                        rampDuration: card.querySelector(`input[name="rampduration_${ifaceName}"]`)?.value
                    };
                }
            });
            
            localStorage.setItem(CONFIG_STORAGE_KEY, JSON.stringify(config));
            console.log('Config saved to localStorage');
        } catch (err) {
            console.error('Failed to save config:', err);
        }
    }

    function restoreConfigFromStorage() {
        try {
            const stored = localStorage.getItem(CONFIG_STORAGE_KEY);
            if (!stored) return;
            
            const config = JSON.parse(stored);
            console.log('Restoring config from localStorage');
            
            // Restore basic fields
            if (config.duration) document.getElementById('duration').value = config.duration;
            if (config.pollInterval) document.getElementById('poll_interval').value = config.pollInterval;
            if (config.preTestTime) document.getElementById('pre_test_time').value = config.preTestTime;
            if (config.postTestTime) document.getElementById('post_test_time').value = config.postTestTime;
            if (config.powerYMin) {
                document.getElementById('power_y_min').value = config.powerYMin;
                // Trigger change event to update chart
                document.getElementById('power_y_min').dispatchEvent(new Event('change'));
            }
            if (config.targetIP) document.getElementById('target_ip').value = config.targetIP;
            if (config.targetPort) document.getElementById('target_port').value = config.targetPort;
            if (config.protocol) document.getElementById('protocol').value = config.protocol;
            if (config.packetSize) document.getElementById('packet_size').value = config.packetSize;
            
            // Restore load enabled checkbox
            if (typeof config.loadEnabled === 'boolean') {
                document.getElementById('load_enabled').checked = config.loadEnabled;
                updateLoadConfigVisibility();
            }
            
            // Store interface configs for later application after interfaces load
            window._pendingInterfaceConfigs = config.interfaceConfigs;
        } catch (err) {
            console.error('Failed to restore config:', err);
        }
    }

    function applyInterfaceConfigs() {
        const configs = window._pendingInterfaceConfigs;
        if (!configs) return;
        
        document.querySelectorAll('.interface-config-card').forEach(card => {
            const ifaceName = card.dataset.iface;
            const savedConfig = configs[ifaceName];
            if (!savedConfig) return;
            
            // Apply enabled state
            const checkbox = card.querySelector('input[type="checkbox"]');
            if (checkbox && typeof savedConfig.enabled === 'boolean') {
                checkbox.checked = savedConfig.enabled;
                card.classList.toggle('enabled', savedConfig.enabled);
            }
            
            // Apply settings
            if (savedConfig.workers) {
                const input = card.querySelector(`input[name="workers_${ifaceName}"]`);
                if (input) input.value = savedConfig.workers;
            }
            if (savedConfig.throughput) {
                const input = card.querySelector(`input[name="throughput_${ifaceName}"]`);
                if (input) input.value = savedConfig.throughput;
            }
            if (savedConfig.rampSteps) {
                const input = card.querySelector(`input[name="ramp_${ifaceName}"]`);
                if (input) input.value = savedConfig.rampSteps;
            }
            if (savedConfig.preTime) {
                const input = card.querySelector(`input[name="pretime_${ifaceName}"]`);
                if (input) input.value = savedConfig.preTime;
            }
            if (savedConfig.rampDuration) {
                const input = card.querySelector(`input[name="rampduration_${ifaceName}"]`);
                if (input) input.value = savedConfig.rampDuration;
            }
        });
        
        delete window._pendingInterfaceConfigs;
    }

    // Restore config on page load (basic fields)
    restoreConfigFromStorage();

    function updateLoadConfigVisibility() {
        if (loadEnabledCheckbox && loadConfigDiv) {
            loadConfigDiv.style.display = loadEnabledCheckbox.checked ? 'block' : 'none';
            if (loadEnabledCheckbox.checked) {
                loadInterfaces();
            }
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

    // Phase transition tracking
    let phaseAnnotations = {};
    
    // Power chart Y-axis minimum (can be updated from UI)
    let powerYMin = 0;
    const powerYMinInput = document.getElementById('power_y_min');
    
    // Update power Y-min when input changes
    if (powerYMinInput) {
        powerYMinInput.addEventListener('change', () => {
            powerYMin = parseInt(powerYMinInput.value) || 0;
            if (powerChart.options.scales.y.min !== powerYMin) {
                powerChart.options.scales.y.min = powerYMin > 0 ? powerYMin : undefined;
                powerChart.options.scales.y.beginAtZero = powerYMin === 0;
                powerChart.update();
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
                    title: { display: true, text: 'Power (mW)' },
                    min: undefined // Will be set dynamically
                }
            },
            animation: {
                duration: 0 // Disable animation for better performance with real-time data
            },
            plugins: {
                annotation: {
                    annotations: {}
                }
            }
        }
    });

    // Initialize Throughput Chart with dynamic multi-interface support
    const interfaceColors = [
        { border: 'rgb(255, 99, 132)', bg: 'rgba(255, 99, 132, 0.1)' },
        { border: 'rgb(54, 162, 235)', bg: 'rgba(54, 162, 235, 0.1)' },
        { border: 'rgb(255, 206, 86)', bg: 'rgba(255, 206, 86, 0.1)' },
        { border: 'rgb(75, 192, 192)', bg: 'rgba(75, 192, 192, 0.1)' },
        { border: 'rgb(153, 102, 255)', bg: 'rgba(153, 102, 255, 0.1)' },
        { border: 'rgb(255, 159, 64)', bg: 'rgba(255, 159, 64, 0.1)' },
    ];
    
    // Track known interfaces for dynamic dataset creation
    let knownInterfaces = new Set();
    
    const throughputChart = new Chart(throughputCtx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [{
                label: 'Total Throughput (Mbps)',
                data: [],
                borderColor: 'rgb(128, 128, 128)',
                backgroundColor: 'rgba(128, 128, 128, 0.1)',
                borderWidth: 2,
                borderDash: [5, 5],
                fill: false,
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
                    title: { display: true, text: 'Throughput (Mbps)' }
                    // Auto-scales based on data
                }
            },
            animation: {
                duration: 0
            },
            plugins: {
                annotation: {
                    annotations: {}
                },
                legend: {
                    display: true,
                    position: 'top'
                }
            }
        }
    });

    // Initialize Preview Chart (for expected throughput profile)
    const previewChart = new Chart(previewCtx, {
        type: 'line',
        data: {
            labels: [],
            datasets: []
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
                    title: { display: true, text: 'Expected Throughput (Mbps)' }
                }
            },
            animation: {
                duration: 0
            },
            plugins: {
                legend: {
                    display: true,
                    position: 'top'
                },
                tooltip: {
                    mode: 'index',
                    intersect: false
                }
            }
        }
    });

    // Helper to get or create dataset for an interface
    function getOrCreateInterfaceDataset(ifaceName) {
        const displayName = ifaceName || 'default';
        let datasetIndex = throughputChart.data.datasets.findIndex(ds => ds.label === displayName);
        
        if (datasetIndex === -1) {
            const colorIndex = knownInterfaces.size % interfaceColors.length;
            const color = interfaceColors[colorIndex];
            knownInterfaces.add(displayName);
            
            throughputChart.data.datasets.push({
                label: displayName,
                data: [],
                borderColor: color.border,
                backgroundColor: color.bg,
                fill: true,
                tension: 0.1
            });
            datasetIndex = throughputChart.data.datasets.length - 1;
        }
        
        return datasetIndex;
    }

    let eventSource = null;
    let startTime = null;
    let collectedData = [];
    let currentPhase = '';

    // Phase colors for charts
    const phaseColors = {
        'pre': { border: 'rgba(255, 193, 7, 0.8)', bg: 'rgba(255, 193, 7, 0.2)' },
        'load': { border: 'rgba(75, 192, 192, 0.8)', bg: 'rgba(75, 192, 192, 0.2)' },
        'post': { border: 'rgba(108, 117, 125, 0.8)', bg: 'rgba(108, 117, 125, 0.2)' }
    };

    // Add phase annotation
    function addPhaseAnnotation(phase, elapsedSeconds) {
        const phaseNames = { 'pre': 'Pre-Test', 'load': 'Load Test', 'post': 'Post-Test' };
        const colors = phaseColors[phase] || phaseColors['load'];
        
        const annotation = {
            type: 'line',
            xMin: elapsedSeconds,
            xMax: elapsedSeconds,
            borderColor: colors.border,
            borderWidth: 2,
            borderDash: [5, 5],
            label: {
                display: true,
                content: phaseNames[phase] || phase,
                position: 'start'
            }
        };

        const annotationId = `phase_${phase}_${elapsedSeconds}`;
        powerChart.options.plugins.annotation.annotations[annotationId] = annotation;
        throughputChart.options.plugins.annotation.annotations[annotationId] = { ...annotation };
    }

    // Event colors by type
    const eventColors = {
        'phase': { border: 'rgba(75, 192, 192, 0.8)', dash: [5, 5] },
        'ramp': { border: 'rgba(255, 159, 64, 0.8)', dash: [3, 3] },
        'iface_start': { border: 'rgba(54, 162, 235, 0.8)', dash: [2, 2] },
        'iface_stop': { border: 'rgba(153, 102, 255, 0.8)', dash: [2, 2] },
        'custom': { border: 'rgba(255, 99, 132, 1)', dash: [] }
    };

    // Add event annotation to charts
    function addEventAnnotation(evt, elapsedSeconds) {
        const colors = eventColors[evt.type] || eventColors['custom'];
        
        // Shorten label for compact display
        let label = evt.message;
        if (label.length > 30) {
            label = label.substring(0, 28) + '...';
        }
        
        const annotation = {
            type: 'line',
            xMin: elapsedSeconds,
            xMax: elapsedSeconds,
            borderColor: colors.border,
            borderWidth: evt.type === 'custom' ? 3 : 1.5,
            borderDash: colors.dash,
            label: {
                display: true,
                content: label,
                position: 'end',
                backgroundColor: 'rgba(255, 255, 255, 0.9)',
                color: '#333',
                font: { size: 10 },
                padding: 3
            }
        };

        const annotationId = `evt_${evt.type}_${Date.now()}_${Math.random().toString(36).substr(2, 5)}`;
        powerChart.options.plugins.annotation.annotations[annotationId] = annotation;
        throughputChart.options.plugins.annotation.annotations[annotationId] = { ...annotation };
        
        // Update charts to show annotation immediately
        powerChart.update();
        throughputChart.update();
    }

    // Progress tracking variables
    let testStartTime = null;
    let testTotalDuration = 0; // in seconds
    let eventTimeline = []; // { time: seconds, description: string }
    
    // Parse duration strings (e.g., "10m", "2m30s", "30s")
    const parseDuration = (str) => {
        let seconds = 0;
        const mins = str.match(/(\d+)m/);
        const secs = str.match(/(\d+)s/);
        if (mins) seconds += parseInt(mins[1]) * 60;
        if (secs) seconds += parseInt(secs[1]);
        return seconds;
    };
    
    // Generate expected throughput profile for preview
    function generateExpectedThroughputProfile() {
        const config = getCurrentConfig();
        
        if (!config.loadEnabled || !config.interfaceConfigs || config.interfaceConfigs.length === 0) {
            return null;
        }
        
        const preTest = parseDuration(config.preTestTime || '0s');
        const loadTest = parseDuration(config.duration || '0s');
        const postTest = parseDuration(config.postTestTime || '0s');
        const totalDuration = preTest + loadTest + postTest;
        
        // Generate time points (every second)
        const timePoints = [];
        const sampleRate = Math.max(1, Math.floor(totalDuration / 500)); // Max 500 points
        for (let t = 0; t <= totalDuration; t += sampleRate) {
            timePoints.push(t);
        }
        if (timePoints[timePoints.length - 1] !== totalDuration) {
            timePoints.push(totalDuration);
        }
        
        // Calculate throughput for each interface at each time point
        const interfaceProfiles = new Map();
        const totalProfile = new Array(timePoints.length).fill(0);
        
        config.interfaceConfigs.forEach(ic => {
            const profile = new Array(timePoints.length).fill(0);
            const targetThroughput = parseFloat(ic.throughput) || 0;
            const rampSteps = parseInt(ic.rampSteps) || 0;
            const preTime = parseDuration(ic.preTime || '0s');
            const rampDuration = parseDuration(ic.rampDuration || '0s') || (rampSteps > 0 ? rampSteps * 5 : 0);
            
            const interfaceStart = preTest + preTime;
            const rampEnd = interfaceStart + rampDuration;
            const loadEnd = preTest + loadTest;
            
            timePoints.forEach((t, i) => {
                if (t < interfaceStart) {
                    profile[i] = 0; // Pre-delay
                } else if (rampSteps > 0 && t < rampEnd) {
                    // Ramping phase
                    const rampProgress = (t - interfaceStart) / rampDuration;
                    profile[i] = targetThroughput * rampProgress;
                } else if (t >= interfaceStart && t < loadEnd) {
                    // Full load phase
                    profile[i] = targetThroughput;
                } else {
                    // Post-test or after load
                    profile[i] = 0;
                }
                
                totalProfile[i] += profile[i];
            });
            
            interfaceProfiles.set(ic.name, profile);
        });
        
        return {
            timePoints,
            interfaceProfiles,
            totalProfile
        };
    }
    
    // Update preview chart with expected throughput
    function updatePreviewChart() {
        const profile = generateExpectedThroughputProfile();
        
        if (!profile) {
            previewChart.data.labels = [];
            previewChart.data.datasets = [];
            previewChart.update();
            return;
        }
        
        // Update chart data
        previewChart.data.labels = profile.timePoints;
        previewChart.data.datasets = [];
        
        // Add total throughput line (dashed)
        previewChart.data.datasets.push({
            label: 'Total Expected',
            data: profile.totalProfile,
            borderColor: 'rgb(128, 128, 128)',
            backgroundColor: 'transparent',
            borderWidth: 2,
            borderDash: [5, 5],
            fill: false,
            tension: 0.1,
            pointRadius: 0
        });
        
        // Add per-interface lines
        let colorIndex = 0;
        profile.interfaceProfiles.forEach((data, ifaceName) => {
            const color = interfaceColors[colorIndex % interfaceColors.length];
            previewChart.data.datasets.push({
                label: ifaceName,
                data: data,
                borderColor: color.border,
                backgroundColor: color.bg,
                borderWidth: 2,
                fill: true,
                tension: 0.1,
                pointRadius: 0
            });
            colorIndex++;
        });
        
        previewChart.update();
    }
    
    // Toggle preview chart visibility
    if (togglePreviewBtn) {
        togglePreviewBtn.addEventListener('click', () => {
            const isVisible = previewChartContainer.style.display !== 'none';
            if (isVisible) {
                previewChartContainer.style.display = 'none';
                togglePreviewBtn.textContent = 'Show Preview';
            } else {
                updatePreviewChart();
                previewChartContainer.style.display = 'block';
                togglePreviewBtn.textContent = 'Hide Preview';
            }
        });
    }
    
    // Update preview when configuration changes (debounced)
    let previewUpdateTimeout = null;
    function schedulePreviewUpdate() {
        if (previewChartContainer.style.display !== 'none') {
            clearTimeout(previewUpdateTimeout);
            previewUpdateTimeout = setTimeout(updatePreviewChart, 500);
        }
    }
    
    // Listen for config changes
    if (form) {
        form.addEventListener('input', schedulePreviewUpdate);
    }
    
    // Calculate total duration and build event timeline
    function updateProgressTracking(config) {
        const preTest = parseDuration(config.preTestTime || '0s');
        const loadTest = parseDuration(config.duration || '0s');
        const postTest = parseDuration(config.postTestTime || '0s');
        testTotalDuration = preTest + loadTest + postTest;
        
        console.log('Progress tracking initialized:', {
            preTest, loadTest, postTest, 
            totalDuration: testTotalDuration,
            config: config
        });
        
        // Build event timeline
        eventTimeline = [];
        let currentTime = 0;
        
        if (preTest > 0) {
            eventTimeline.push({ time: 0, description: 'Pre-Test Baseline Start' });
            currentTime = preTest;
            eventTimeline.push({ time: currentTime, description: 'Load Test Start' });
        } else {
            eventTimeline.push({ time: 0, description: 'Load Test Start' });
        }
        
        // Add interface start events and ramp events
        if (config.loadEnabled && config.interfaceConfigs) {
            config.interfaceConfigs.forEach(ic => {
                const preTime = parseDuration(ic.preTime || '0s');
                const rampSteps = parseInt(ic.rampSteps) || 0;
                const rampDuration = parseDuration(ic.rampDuration || '0s') || (rampSteps * 5);
                
                const startTime = preTest + preTime;
                eventTimeline.push({ time: startTime, description: `${ic.name} Start` });
                
                if (rampSteps > 0) {
                    const stepDuration = rampDuration / rampSteps;
                    for (let i = 1; i <= rampSteps; i++) {
                        const rampTime = startTime + (stepDuration * i);
                        const target = (parseFloat(ic.throughput) / rampSteps * i).toFixed(0);
                        eventTimeline.push({ time: rampTime, description: `${ic.name} Ramp ${i}/${rampSteps} (${target} Mbps)` });
                    }
                }
            });
        }
        
        currentTime = preTest + loadTest;
        if (postTest > 0) {
            eventTimeline.push({ time: currentTime, description: 'Post-Test Baseline Start' });
            currentTime += postTest;
            eventTimeline.push({ time: currentTime, description: 'Test Complete' });
        } else {
            eventTimeline.push({ time: currentTime, description: 'Test Complete' });
        }
        
        // Sort timeline by time
        eventTimeline.sort((a, b) => a.time - b.time);
        
        testStartTime = Date.now();
    }
    
    // Update progress UI
    function updateProgressUI(elapsedSeconds, phaseName) {
        const progressPercent = Math.min(100, (elapsedSeconds / testTotalDuration * 100).toFixed(1));
        const remainingSeconds = Math.max(0, testTotalDuration - elapsedSeconds);
        
        // Format time as MM:SS
        const formatTime = (seconds) => {
            const mins = Math.floor(seconds / 60);
            const secs = Math.floor(seconds % 60);
            return `${mins}:${secs.toString().padStart(2, '0')}`;
        };
        
        // Update progress bar and text
        document.getElementById('progressBar').style.width = `${progressPercent}%`;
        document.getElementById('progressPercent').textContent = progressPercent;
        document.getElementById('progressTime').textContent = formatTime(elapsedSeconds);
        document.getElementById('progressTotal').textContent = formatTime(testTotalDuration);
        document.getElementById('timeRemaining').textContent = formatTime(remainingSeconds);
        
        // Map phase code to display name
        const phaseNames = { 'pre': 'Pre-Test Baseline', 'load': 'Load Test', 'post': 'Post-Test Baseline' };
        document.getElementById('currentPhase').textContent = phaseNames[phaseName] || phaseName || '--';
        
        // Find next upcoming events (up to 3)
        const upcomingEvents = eventTimeline
            .filter(evt => evt.time > elapsedSeconds)
            .slice(0, 3)
            .map(evt => {
                const timeUntil = evt.time - elapsedSeconds;
                return `${evt.description} (in ${formatTime(timeUntil)})`;
            });
        
        const eventsText = upcomingEvents.length > 0 
            ? upcomingEvents.join(' • ') 
            : 'No more events';
        document.getElementById('eventsList').textContent = eventsText;
    }

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
                // Add phase transition marker
                if (currentPhase !== '') {
                    addPhaseAnnotation(phase, elapsedSeconds);
                }
                currentPhase = phase;
                const phaseNames = { 'pre': 'Pre-Test Baseline', 'load': 'Load Test', 'post': 'Post-Test Baseline' };
                statusDiv.textContent = `Status: Running - ${phaseNames[phase] || phase}`;
            }

            // Update Power Chart
            powerChart.data.labels.push(elapsedSeconds);
            powerChart.data.datasets[0].data.push(data.power_mw);
            powerChart.update();

            // Update Throughput Chart with per-interface data
            const throughputMbps = data.throughput_mbps || 0;
            const throughputByInterface = data.throughput_by_interface || {};
            const targetThroughputByInterface = data.target_throughput_by_interface || {};
            
            // Add label (time) to chart
            throughputChart.data.labels.push(elapsedSeconds);
            
            // Calculate total target throughput for this data point
            const totalTarget = Object.values(targetThroughputByInterface).reduce((sum, val) => sum + val, 0);
            
            // Update total throughput (first dataset, dashed line)
            throughputChart.data.datasets[0].data.push(throughputMbps);
            
            // Update per-interface throughput datasets (actual throughput)
            for (const [ifaceName, ifaceThroughput] of Object.entries(throughputByInterface)) {
                const datasetIndex = getOrCreateInterfaceDataset(ifaceName);
                // Backfill with nulls if this interface was added late
                while (throughputChart.data.datasets[datasetIndex].data.length < throughputChart.data.labels.length - 1) {
                    throughputChart.data.datasets[datasetIndex].data.push(null);
                }
                throughputChart.data.datasets[datasetIndex].data.push(ifaceThroughput);
            }
            
            // Add/update target throughput datasets (dotted lines)
            for (const [ifaceName, targetThroughput] of Object.entries(targetThroughputByInterface)) {
                const targetDatasetLabel = `${ifaceName} (target)`;
                let targetDatasetIndex = throughputChart.data.datasets.findIndex(ds => ds.label === targetDatasetLabel);
                
                if (targetDatasetIndex === -1) {
                    // Create new target dataset with dotted line
                    const actualDatasetIndex = throughputChart.data.datasets.findIndex(ds => ds.label === ifaceName);
                    const color = actualDatasetIndex >= 0 ? throughputChart.data.datasets[actualDatasetIndex].borderColor : 'rgba(150, 150, 150, 0.6)';
                    
                    throughputChart.data.datasets.push({
                        label: targetDatasetLabel,
                        data: [],
                        borderColor: color,
                        backgroundColor: 'transparent',
                        fill: false,
                        borderWidth: 1,
                        borderDash: [5, 5],
                        pointRadius: 0,
                        tension: 0
                    });
                    targetDatasetIndex = throughputChart.data.datasets.length - 1;
                }
                
                // Backfill with nulls if needed
                while (throughputChart.data.datasets[targetDatasetIndex].data.length < throughputChart.data.labels.length - 1) {
                    throughputChart.data.datasets[targetDatasetIndex].data.push(null);
                }
                throughputChart.data.datasets[targetDatasetIndex].data.push(targetThroughput);
            }
            
            // Ensure all datasets have same length (fill with null for missing data)
            for (let i = 0; i < throughputChart.data.datasets.length; i++) {
                while (throughputChart.data.datasets[i].data.length < throughputChart.data.labels.length) {
                    throughputChart.data.datasets[i].data.push(null);
                }
            }
            
            throughputChart.update();

            // Update throughput display
            throughputValueDiv.textContent = throughputMbps.toFixed(1);

            // Process events and add annotations
            const events = data.events || [];
            events.forEach(evt => {
                addEventAnnotation(evt, elapsedSeconds);
            });
            
            // Store for CSV export with phase info and per-interface data
            const dataPoint = {
                timestamp: data.timestamp,
                elapsed_seconds: elapsedSeconds,
                power_mw: data.power_mw,
                throughput_mbps: throughputMbps,
                throughput_by_interface: throughputByInterface,
                target_throughput_by_interface: data.target_throughput_by_interface || {},
                phase: phase,
                events: events
            };
            collectedData.push(dataPoint);
            
            // Update progress UI
            if (testTotalDuration > 0) {
                updateProgressUI(elapsedSeconds, phase);
            }
            
            // Debug logging
            if (collectedData.length % 10 === 1) {
                console.log(`Collected ${collectedData.length} data points. Latest:`, dataPoint);
            }
        };

        eventSource.addEventListener('done', function(e) {
            statusDiv.textContent = 'Status: Test Finished';
            startBtn.disabled = false;
            stopBtn.disabled = true;
            downloadBtn.disabled = false;
            if (saveTestBtn) saveTestBtn.disabled = false;
            if (markerSection) markerSection.classList.remove('active');
            
            // Hide progress section
            const progressSection = document.getElementById('progressSection');
            if (progressSection) progressSection.style.display = 'none';
            
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
        
        // Save config to localStorage before starting
        saveConfigToStorage();
        
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
                if (saveTestBtn) saveTestBtn.disabled = true;
                if (markerSection) markerSection.classList.add('active');
                
                // Reset charts and data
                powerChart.data.labels = [];
                powerChart.data.datasets[0].data = [];
                powerChart.options.plugins.annotation.annotations = {};
                
                // Apply power Y-axis minimum from input
                const yMinValue = parseInt(document.getElementById('power_y_min')?.value) || 0;
                powerChart.options.scales.y.min = yMinValue > 0 ? yMinValue : undefined;
                powerChart.options.scales.y.beginAtZero = yMinValue === 0;
                powerChart.update();
                
                // Reset throughput chart - remove all dynamic interface datasets
                // Keep only the first dataset (Total) but clear its data
                throughputChart.data.labels = [];
                throughputChart.data.datasets.length = 1; // Keep only Total
                throughputChart.data.datasets[0].data = [];
                throughputChart.data.datasets[0] = {
                    label: 'Total Throughput (Mbps)',
                    data: [],
                    borderColor: 'rgb(128, 128, 128)',
                    backgroundColor: 'rgba(128, 128, 128, 0.1)',
                    borderWidth: 2,
                    borderDash: [5, 5],
                    fill: false,
                    tension: 0.1
                };
                throughputChart.options.plugins.annotation.annotations = {};
                knownInterfaces.clear();
                throughputChart.update('none'); // Force full update
                
                throughputValueDiv.textContent = '0.0';
                
                collectedData = [];
                startTime = null;
                currentPhase = '';
                
                // Show progress section and initialize
                const progressSection = document.getElementById('progressSection');
                if (progressSection) {
                    progressSection.style.display = 'block';
                    // Calculate total duration and build event timeline
                    const testConfig = getCurrentConfig();
                    updateProgressTracking(testConfig);
                }

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
            if (saveTestBtn) saveTestBtn.disabled = false;
            if (markerSection) markerSection.classList.remove('active');
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
        console.log('Download clicked. collectedData length:', collectedData.length);
        if (collectedData.length === 0) {
            alert("No data to export");
            return;
        }
        console.log('First data point:', collectedData[0]);
        console.log('Last data point:', collectedData[collectedData.length - 1]);

        // Get test configuration using getCurrentConfig
        const config = getCurrentConfig();

        // Build interface config summary
        let interfaceSummary = 'OS Routing';
        if (config.interfaceConfigs && config.interfaceConfigs.length > 0) {
            interfaceSummary = config.interfaceConfigs.map(ic => 
                `${ic.name}(w:${ic.workers},t:${ic.throughput}Mbps,r:${ic.rampSteps})`
            ).join('; ');
        }

        // Build metadata header
        const metadata = [
            "# Power Consumption Test Report",
            `# Generated: ${new Date().toISOString()}`,
            `# Duration: ${config.duration}`,
            `# Poll Interval: ${config.pollInterval}`,
            `# Pre-Test Baseline: ${config.preTestTime}`,
            `# Post-Test Baseline: ${config.postTestTime}`,
            `# Load Enabled: ${config.loadEnabled}`,
            config.loadEnabled ? `# Target: ${config.targetIP}:${config.targetPort}` : "",
            config.loadEnabled ? `# Protocol: ${config.protocol}` : "",
            config.loadEnabled ? `# Packet Size: ${config.packetSize}` : "",
            config.loadEnabled ? `# Interface Configs: ${interfaceSummary}` : "",
            "#",
        ].filter(line => line !== "").join("\n");

        // Collect all unique interface names from data
        const allInterfaces = new Set();
        collectedData.forEach(e => {
            if (e.throughput_by_interface) {
                Object.keys(e.throughput_by_interface).forEach(iface => allInterfaces.add(iface));
            }
        });
        const interfaceList = Array.from(allInterfaces).sort();

        // Build CSV header with dynamic interface columns
        let csvHeader = "Timestamp,ElapsedSeconds,PowerMW,ThroughputTotalMbps,TargetThroughputTotalMbps";
        interfaceList.forEach(iface => {
            csvHeader += `,Throughput_${iface}_Mbps,Target_${iface}_Mbps`;
        });
        csvHeader += ",Phase,Events";

        // Build CSV rows
        const csvRows = collectedData.map(e => {
            // Calculate total target throughput
            const targetTotal = interfaceList.reduce((sum, iface) => {
                return sum + ((e.target_throughput_by_interface && e.target_throughput_by_interface[iface]) || 0);
            }, 0);
            
            let row = `${e.timestamp},${e.elapsed_seconds},${e.power_mw},${e.throughput_mbps},${targetTotal}`;
            interfaceList.forEach(iface => {
                const ifaceThroughput = (e.throughput_by_interface && e.throughput_by_interface[iface]) || 0;
                const ifaceTarget = (e.target_throughput_by_interface && e.target_throughput_by_interface[iface]) || 0;
                row += `,${ifaceThroughput},${ifaceTarget}`;
            });
            // Format events as pipe-separated list and escape for CSV
            const eventsStr = (e.events || []).map(evt => `[${evt.type}] ${evt.message}`).join(' | ');
            row += `,${e.phase},"${eventsStr.replace(/"/g, '""')}"`;
            return row;
        }).join("\n");

        console.log('Live CSV: rows generated:', collectedData.length);
        console.log('Live CSV: csvRows length:', csvRows.length);
        
        const csvContent = metadata + "\n" + csvHeader + "\n" + csvRows;
        
        console.log('Live CSV: total content length:', csvContent.length);

        // Use Blob API for reliable file download
        const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.setAttribute("href", url);
        
        // Generate filename with timestamp
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        link.setAttribute("download", `power_test_${timestamp}.csv`);
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        
        // Clean up the blob URL
        setTimeout(() => URL.revokeObjectURL(url), 100);
    });

    // ============ Test History Management ============
    const historyListDiv = document.getElementById('historyList');
    const saveTestBtn = document.getElementById('saveTestBtn');
    const clearHistoryBtn = document.getElementById('clearHistoryBtn');

    // ============ Custom Markers ============
    const markerSection = document.getElementById('markerSection');
    const markerTextInput = document.getElementById('markerText');
    const addMarkerBtn = document.getElementById('addMarkerBtn');
    const markerFeedback = document.getElementById('markerFeedback');

    // Add custom marker during test
    if (addMarkerBtn) {
        addMarkerBtn.addEventListener('click', async () => {
            const message = markerTextInput.value.trim();
            if (!message) {
                alert('Please enter marker text');
                return;
            }

            try {
                addMarkerBtn.disabled = true;
                const formData = new FormData();
                formData.append('message', message);
                
                const response = await fetch('/marker', {
                    method: 'POST',
                    body: formData
                });

                if (response.ok) {
                    markerTextInput.value = '';
                    // Show feedback
                    markerFeedback.style.display = 'block';
                    setTimeout(() => {
                        markerFeedback.style.display = 'none';
                    }, 2000);
                } else {
                    const errorText = await response.text();
                    alert('Failed to add marker: ' + errorText);
                }
            } catch (error) {
                console.error('Error adding marker:', error);
                alert('Error adding marker');
            } finally {
                addMarkerBtn.disabled = false;
            }
        });

        // Allow Enter key to submit marker
        if (markerTextInput) {
            markerTextInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    addMarkerBtn.click();
                }
            });
        }
    }

    // Build config object for saving
    function getCurrentConfig() {
        // Collect per-interface configs
        const interfaceConfigs = [];
        document.querySelectorAll('.interface-config-card.enabled').forEach(card => {
            const ifaceName = card.dataset.iface;
            interfaceConfigs.push({
                name: ifaceName,
                workers: card.querySelector(`input[name="workers_${ifaceName}"]`)?.value || '16',
                throughput: card.querySelector(`input[name="throughput_${ifaceName}"]`)?.value || '0',
                rampSteps: card.querySelector(`input[name="ramp_${ifaceName}"]`)?.value || '0',
                preTime: card.querySelector(`input[name="pretime_${ifaceName}"]`)?.value || '0s',
                rampDuration: card.querySelector(`input[name="rampduration_${ifaceName}"]`)?.value || '0s'
            });
        });

        return {
            duration: document.getElementById('duration').value,
            pollInterval: document.getElementById('poll_interval').value,
            preTestTime: document.getElementById('pre_test_time').value,
            postTestTime: document.getElementById('post_test_time').value,
            loadEnabled: document.getElementById('load_enabled').checked,
            targetIP: document.getElementById('target_ip').value,
            targetPort: document.getElementById('target_port').value,
            protocol: document.getElementById('protocol').value,
            packetSize: document.getElementById('packet_size').value,
            interfaceConfigs: interfaceConfigs
        };
    }

    // Render history list
    async function renderHistoryList() {
        try {
            const tests = await getAllTests();
            if (tests.length === 0) {
                historyListDiv.innerHTML = '<em>No saved tests</em>';
                return;
            }

            // Sort by timestamp descending (newest first)
            tests.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

            historyListDiv.innerHTML = tests.map(test => {
                const date = new Date(test.timestamp);
                const dateStr = date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
                const dataPoints = test.data?.length || 0;
                const loadInfo = test.config?.loadEnabled ? 
                    `${test.config.protocol?.toUpperCase() || 'UDP'} → ${test.config.targetIP || 'N/A'}` : 
                    'No load';
                
                return `
                    <div class="history-item" data-id="${test.id}">
                        <div class="history-info">
                            <div class="title">${dateStr}</div>
                            <div class="details">
                                ${dataPoints} data points | ${test.config?.duration || 'N/A'} | ${loadInfo}
                            </div>
                        </div>
                        <div class="history-actions">
                            <button class="btn-small history-load" title="Load into charts">📊 Load</button>
                            <button class="btn-small history-download" title="Download CSV">💾 CSV</button>
                            <button class="btn-small btn-danger history-delete" title="Delete">🗑️</button>
                        </div>
                    </div>
                `;
            }).join('');

            // Add event listeners to history buttons
            historyListDiv.querySelectorAll('.history-item').forEach(item => {
                const id = parseInt(item.dataset.id);

                item.querySelector('.history-load').addEventListener('click', async () => {
                    const test = await getTest(id);
                    if (test && test.data) {
                        loadTestIntoCharts(test);
                    }
                });

                item.querySelector('.history-download').addEventListener('click', async () => {
                    const test = await getTest(id);
                    if (test) {
                        downloadTestAsCSV(test);
                    }
                });

                item.querySelector('.history-delete').addEventListener('click', async () => {
                    if (confirm('Delete this test?')) {
                        await deleteTest(id);
                        renderHistoryList();
                    }
                });
            });
        } catch (error) {
            console.error('Error rendering history:', error);
            historyListDiv.innerHTML = '<em>Error loading history</em>';
        }
    }

    // Load a saved test into the charts
    function loadTestIntoCharts(test) {
        // Clear existing data
        collectedData.length = 0;
        powerChart.data.labels = [];
        powerChart.data.datasets[0].data = [];
        throughputChart.data.labels = [];
        throughputChart.data.datasets[0].data = [];
        
        // Clear dynamic interface datasets from throughput chart (keep only first - Total)
        while (throughputChart.data.datasets.length > 1) {
            throughputChart.data.datasets.pop();
        }
        knownInterfaces.clear();
        
        powerChart.options.plugins.annotation.annotations = {};
        throughputChart.options.plugins.annotation.annotations = {};

        // Load the test data
        let currentPhase = null;
        test.data.forEach((dp, idx) => {
            collectedData.push(dp);
            const label = dp.elapsed_seconds?.toFixed(0) || idx.toString();
            powerChart.data.labels.push(label);
            powerChart.data.datasets[0].data.push(dp.power_mw);
            throughputChart.data.labels.push(label);
            throughputChart.data.datasets[0].data.push(dp.throughput_mbps);

            // Add per-interface data if available
            if (dp.throughput_by_interface) {
                Object.entries(dp.throughput_by_interface).forEach(([ifaceName, ifaceThroughput]) => {
                    const datasetIndex = getOrCreateInterfaceDataset(ifaceName);
                    const dataset = throughputChart.data.datasets[datasetIndex];
                    // Pad with zeros if this interface appeared late
                    while (dataset.data.length < idx) {
                        dataset.data.push(0);
                    }
                    dataset.data.push(ifaceThroughput);
                });
            }
            
            // Ensure all interface datasets have the same length
            throughputChart.data.datasets.forEach((ds, dsIdx) => {
                if (dsIdx > 0) { // Skip the Total dataset
                    while (ds.data.length <= idx) {
                        ds.data.push(0);
                    }
                }
            });

            // Add phase annotations
            if (dp.phase && dp.phase !== currentPhase) {
                currentPhase = dp.phase;
                addPhaseAnnotation(dp.phase, idx);
            }

            // Add event annotations
            if (dp.events && dp.events.length > 0) {
                dp.events.forEach(evt => {
                    addEventAnnotation(evt, parseFloat(dp.elapsed_seconds) || idx);
                });
            }
        });

        powerChart.update();
        throughputChart.update();

        // Update throughput display with last value
        if (test.data.length > 0) {
            const lastPoint = test.data[test.data.length - 1];
            throughputValueDiv.textContent = (lastPoint.throughput_mbps || 0).toFixed(1);
        }

        statusDiv.textContent = `Status: Loaded test from ${new Date(test.timestamp).toLocaleString()}`;
        downloadBtn.disabled = false;
    }

    // Download a saved test as CSV
    function downloadTestAsCSV(test) {
        console.log('downloadTestAsCSV called with test:', test);
        console.log('Test data length:', test?.data?.length);
        
        if (!test || !test.data || test.data.length === 0) {
            alert('No data to export from this test');
            return;
        }
        
        const config = test.config || {};
        
        // Build interface config summary
        let interfaceSummary = 'OS Routing';
        if (config.interfaceConfigs && config.interfaceConfigs.length > 0) {
            interfaceSummary = config.interfaceConfigs.map(ic => 
                `${ic.name}(w:${ic.workers},t:${ic.throughput}Mbps,r:${ic.rampSteps})`
            ).join('; ');
        }

        const metadata = [
            "# Power Consumption Test Report",
            `# Generated: ${test.timestamp}`,
            `# Duration: ${config.duration || 'N/A'}`,
            `# Poll Interval: ${config.pollInterval || 'N/A'}`,
            `# Pre-Test Baseline: ${config.preTestTime || '0s'}`,
            `# Post-Test Baseline: ${config.postTestTime || '0s'}`,
            `# Load Enabled: ${config.loadEnabled || false}`,
            config.loadEnabled ? `# Target: ${config.targetIP}:${config.targetPort}` : "",
            config.loadEnabled ? `# Protocol: ${config.protocol}` : "",
            config.loadEnabled ? `# Packet Size: ${config.packetSize}` : "",
            config.loadEnabled ? `# Interface Configs: ${interfaceSummary}` : "",
            "#",
        ].filter(line => line !== "").join("\n");

        // Collect all unique interface names from data
        const allInterfaces = new Set();
        test.data.forEach(e => {
            if (e.throughput_by_interface) {
                Object.keys(e.throughput_by_interface).forEach(iface => allInterfaces.add(iface));
            }
        });
        const interfaceList = Array.from(allInterfaces).sort();

        // Build CSV header with dynamic interface columns
        let csvHeader = "Timestamp,ElapsedSeconds,PowerMW,ThroughputTotalMbps,TargetThroughputTotalMbps";
        interfaceList.forEach(iface => {
            csvHeader += `,Throughput_${iface}_Mbps,Target_${iface}_Mbps`;
        });
        csvHeader += ",Phase,Events";

        // Build CSV rows
        const csvRows = test.data.map(e => {
            // Calculate total target throughput
            const targetTotal = interfaceList.reduce((sum, iface) => {
                return sum + ((e.target_throughput_by_interface && e.target_throughput_by_interface[iface]) || 0);
            }, 0);
            
            let row = `${e.timestamp},${e.elapsed_seconds},${e.power_mw},${e.throughput_mbps},${targetTotal}`;
            interfaceList.forEach(iface => {
                const ifaceThroughput = (e.throughput_by_interface && e.throughput_by_interface[iface]) || 0;
                const ifaceTarget = (e.target_throughput_by_interface && e.target_throughput_by_interface[iface]) || 0;
                row += `,${ifaceThroughput},${ifaceTarget}`;
            });
            // Format events as pipe-separated list and escape for CSV
            const eventsStr = (e.events || []).map(evt => `[${evt.type}] ${evt.message}`).join(' | ');
            row += `,${e.phase},"${eventsStr.replace(/"/g, '""')}"`;
            return row;
        }).join("\n");

        const csvContent = metadata + "\n" + csvHeader + "\n" + csvRows;
        
        console.log('History CSV content length:', csvContent.length);
        console.log('History CSV rows count:', test.data.length);

        // Use Blob API for reliable file download
        const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.setAttribute("href", url);
        const timestamp = new Date(test.timestamp).toISOString().replace(/[:.]/g, '-').slice(0, 19);
        link.setAttribute("download", `power_test_${timestamp}.csv`);
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        
        // Clean up the blob URL
        setTimeout(() => URL.revokeObjectURL(url), 100);
    }

    // Save current test
    if (saveTestBtn) {
        saveTestBtn.addEventListener('click', async () => {
            if (collectedData.length === 0) {
                alert('No test data to save');
                return;
            }

            try {
                const testData = {
                    timestamp: new Date().toISOString(),
                    config: getCurrentConfig(),
                    data: [...collectedData]
                };
                await saveTest(testData);
                renderHistoryList();
                alert('Test saved successfully');
            } catch (error) {
                console.error('Error saving test:', error);
                alert('Failed to save test');
            }
        });
    }

    // Clear all history
    if (clearHistoryBtn) {
        clearHistoryBtn.addEventListener('click', async () => {
            if (confirm('Delete ALL saved tests? This cannot be undone.')) {
                try {
                    const tests = await getAllTests();
                    for (const test of tests) {
                        await deleteTest(test.id);
                    }
                    renderHistoryList();
                } catch (error) {
                    console.error('Error clearing history:', error);
                }
            }
        });
    }

    // Initial render of history list
    renderHistoryList();
});
