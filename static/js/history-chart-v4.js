/*
 * 星垣 - 历史数据图表脚本（多图表版本）
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

// ==================== CPU 图表相关 ====================
let cpuChart = null;
let cpuAutoRefreshTimer = null;
let cpuCurrentStartTime = null;
let cpuCurrentEndTime = null;

// ==================== 内存 图表相关 ====================
let memoryChart = null;
let memoryAutoRefreshTimer = null;
let memoryCurrentStartTime = null;
let memoryCurrentEndTime = null;

// ==================== 磁盘 图表相关 ====================
let diskChart = null;
let diskAutoRefreshTimer = null;
let diskCurrentStartTime = null;
let diskCurrentEndTime = null;

// ==================== 通用工具函数 ====================

// 格式化时间为 datetime-local 格式（用于输入框显示，只需要分钟）
function formatDateTimeLocal(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day}T${hours}:${minutes}`;
}

// 格式化时间为完整的数据库查询格式（包含秒）
function formatDateTimeForQuery(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    const seconds = String(date.getSeconds()).padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
}

// 智能转换字节单位（KB -> MB -> GB）
function formatBytes(kb, decimals = 2) {
    if (kb === 0) return '0 KB/s';
    
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['KB/s', 'MB/s', 'GB/s', 'TB/s'];
    
    let i = 0;
    let value = kb;
    
    // 自动选择合适的单位
    while (value >= k && i < sizes.length - 1) {
        value /= k;
        i++;
    }
    
    return value.toFixed(dm) + ' ' + sizes[i];
}

// 获取最佳单位索引（用于Y轴单位统一）
function getBestUnit(maxValue) {
    if (maxValue === 0) return { unit: 'KB/s', divider: 1, index: 0 };
    
    const k = 1024;
    const sizes = ['KB/s', 'MB/s', 'GB/s', 'TB/s'];
    
    let i = 0;
    let value = maxValue;
    
    while (value >= k && i < sizes.length - 1) {
        value /= k;
        i++;
    }
    
    return { 
        unit: sizes[i], 
        divider: Math.pow(k, i),
        index: i 
    };
}

// ==================== CPU 图表函数 ====================

// 初始化CPU时间选择器（默认1分钟）
function initCPUTimeSelector() {
    const now = new Date();
    const oneMinuteAgo = new Date(now.getTime() - 60 * 1000);
    
    cpuCurrentEndTime = now;
    cpuCurrentStartTime = oneMinuteAgo;
    
    document.getElementById('cpu-end-time').value = formatDateTimeLocal(now);
    document.getElementById('cpu-start-time').value = formatDateTimeLocal(oneMinuteAgo);
}

// CPU快速选择时间范围
function cpuSelectQuickTime(minutes) {
    // 移除所有active类
    document.querySelectorAll('#cpu-chart-section .quick-time-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    event.target.classList.add('active');
    
    const now = new Date();
    const past = new Date(now.getTime() - minutes * 60 * 1000);
    
    cpuCurrentEndTime = now;
    cpuCurrentStartTime = past;
    
    document.getElementById('cpu-end-time').value = formatDateTimeLocal(now);
    document.getElementById('cpu-start-time').value = formatDateTimeLocal(past);
    
    loadCPUChartData();
}

// CPU手动查询（停止自动刷新）
function cpuManualQuery() {
    // 停止自动刷新
    if (cpuAutoRefreshTimer) {
        clearInterval(cpuAutoRefreshTimer);
        cpuAutoRefreshTimer = null;
    }
    
    // 获取手动输入的时间
    const startInput = document.getElementById('cpu-start-time').value;
    const endInput = document.getElementById('cpu-end-time').value;
    
    cpuCurrentStartTime = new Date(startInput);
    cpuCurrentEndTime = new Date(endInput);
    
    loadCPUChartData();
}

// 启动CPU自动刷新
function startCPUAutoRefresh() {
    if (cpuAutoRefreshTimer) {
        clearInterval(cpuAutoRefreshTimer);
    }
    
    cpuAutoRefreshTimer = setInterval(async () => {
        const now = new Date();
        const timeRange = cpuCurrentEndTime - cpuCurrentStartTime;
        
        cpuCurrentEndTime = now;
        cpuCurrentStartTime = new Date(now.getTime() - timeRange);
        
        document.getElementById('cpu-end-time').value = formatDateTimeLocal(cpuCurrentEndTime);
        document.getElementById('cpu-start-time').value = formatDateTimeLocal(cpuCurrentStartTime);
        
        await updateCPUChartIncremental();
    }, 1000);
}

// CPU增量更新图表数据
async function updateCPUChartIncremental() {
    try {
        if (!cpuChart) {
            await loadCPUChartData();
            return;
        }
        
        const endTime = formatDateTimeForQuery(cpuCurrentEndTime);
        const threeSecondsAgo = new Date(cpuCurrentEndTime.getTime() - 3000);
        const startTime = formatDateTimeForQuery(threeSecondsAgo);
        
        const url = `/api/history/cpu?start=${encodeURIComponent(startTime)}&end=${encodeURIComponent(endTime)}`;
        const response = await fetch(url);
        
        if (!response.ok) return;
        
        const newData = await response.json();
        if (!newData || newData.length === 0) return;
        
        const lastLabel = cpuChart.data.labels[cpuChart.data.labels.length - 1];
        
        newData.forEach(item => {
            const timeLabel = item.timestamp.substring(11, 19);
            const value = item.usage;
            
            if (timeLabel !== lastLabel && !cpuChart.data.labels.includes(timeLabel)) {
                cpuChart.data.labels.push(timeLabel);
                cpuChart.data.datasets[0].data.push(value);
            }
        });
        
        const timeRangeSeconds = (cpuCurrentEndTime - cpuCurrentStartTime) / 1000;
        const maxDataPoints = Math.ceil(timeRangeSeconds);
        
        while (cpuChart.data.labels.length > maxDataPoints) {
            cpuChart.data.labels.shift();
            cpuChart.data.datasets[0].data.shift();
        }
        
        const pointCount = cpuChart.data.labels.length;
        cpuChart.data.datasets[0].pointRadius = new Array(pointCount).fill(0);
        cpuChart.data.datasets[0].pointHoverRadius = new Array(pointCount).fill(5);
        cpuChart.data.datasets[0].pointHitRadius = new Array(pointCount).fill(10);
        
        cpuChart.update({
            duration: 800,
            easing: 'easeInOutQuart',
            lazy: false
        });
        
        document.getElementById('cpu-data-count').textContent = cpuChart.data.labels.length;
        document.getElementById('cpu-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        console.error('CPU增量更新失败:', error);
    }
}

// 加载CPU图表数据
async function loadCPUChartData() {
    const startTime = formatDateTimeForQuery(cpuCurrentStartTime);
    const endTime = formatDateTimeForQuery(cpuCurrentEndTime);
    
    document.getElementById('cpu-loading').style.display = 'block';
    document.getElementById('cpu-error').style.display = 'none';
    document.getElementById('cpu-chart-container').style.display = 'none';
    
    try {
        const url = `/api/history/cpu?start=${encodeURIComponent(startTime)}&end=${encodeURIComponent(endTime)}`;
        const response = await fetch(url);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const cpuData = await response.json();
        
        if (!cpuData || cpuData.length === 0) {
            throw new Error('查询时间范围内无数据');
        }
        
        document.getElementById('cpu-data-count').textContent = cpuData.length;
        
        renderCPUChart(cpuData);
        
        document.getElementById('cpu-loading').style.display = 'none';
        document.getElementById('cpu-chart-container').style.display = 'block';
        
        document.getElementById('cpu-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        document.getElementById('cpu-loading').style.display = 'none';
        document.getElementById('cpu-error').style.display = 'block';
        document.getElementById('cpu-error').textContent = '加载数据失败: ' + error.message;
    }
}

// 渲染CPU图表
function renderCPUChart(data) {
    const ctx = document.getElementById('cpu-chart');
    
    const labels = data.map(d => d.timestamp.substring(11, 19));
    const values = data.map(d => d.usage);
    
    if (cpuChart) {
        cpuChart.destroy();
    }
    
    cpuChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'CPU使用率',
                data: values,
                borderColor: '#5e72e4',
                backgroundColor: 'rgba(94, 114, 228, 0.1)',
                fill: true,
                tension: 0.4,
                borderWidth: 2,
                pointRadius: 0,
                pointHoverRadius: 5,
                pointHitRadius: 10,
                cubicInterpolationMode: 'monotone'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: {
                duration: 800,
                easing: 'easeInOutQuart'
            },
            transitions: {
                active: {
                    animation: {
                        duration: 400
                    }
                }
            },
            plugins: {
                legend: {
                    display: false
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        label: function(context) {
                            return 'CPU使用率: ' + context.parsed.y.toFixed(2) + '%';
                        }
                    }
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    max: 100,
                    ticks: {
                        callback: function(value) {
                            return value + '%';
                        }
                    },
                    grid: {
                        color: 'rgba(0, 0, 0, 0.05)'
                    }
                },
                x: {
                    ticks: {
                        maxTicksLimit: 12,
                        maxRotation: 0,
                        minRotation: 0
                    },
                    grid: {
                        display: false
                    }
                }
            },
            interaction: {
                mode: 'nearest',
                axis: 'x',
                intersect: false
            }
        }
    });
}

// ==================== 内存 图表函数 ====================

// 初始化内存时间选择器（默认1分钟）
function initMemoryTimeSelector() {
    const now = new Date();
    const oneMinuteAgo = new Date(now.getTime() - 60 * 1000);
    
    memoryCurrentEndTime = now;
    memoryCurrentStartTime = oneMinuteAgo;
    
    document.getElementById('memory-end-time').value = formatDateTimeLocal(now);
    document.getElementById('memory-start-time').value = formatDateTimeLocal(oneMinuteAgo);
}

// 内存快速选择时间范围
function memorySelectQuickTime(minutes) {
    document.querySelectorAll('#memory-chart-section .quick-time-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    event.target.classList.add('active');
    
    const now = new Date();
    const past = new Date(now.getTime() - minutes * 60 * 1000);
    
    memoryCurrentEndTime = now;
    memoryCurrentStartTime = past;
    
    document.getElementById('memory-end-time').value = formatDateTimeLocal(now);
    document.getElementById('memory-start-time').value = formatDateTimeLocal(past);
    
    loadMemoryChartData();
}

// 内存手动查询（停止自动刷新）
function memoryManualQuery() {
    if (memoryAutoRefreshTimer) {
        clearInterval(memoryAutoRefreshTimer);
        memoryAutoRefreshTimer = null;
    }
    
    const startInput = document.getElementById('memory-start-time').value;
    const endInput = document.getElementById('memory-end-time').value;
    
    memoryCurrentStartTime = new Date(startInput);
    memoryCurrentEndTime = new Date(endInput);
    
    loadMemoryChartData();
}

// 启动内存自动刷新
function startMemoryAutoRefresh() {
    if (memoryAutoRefreshTimer) {
        clearInterval(memoryAutoRefreshTimer);
    }
    
    memoryAutoRefreshTimer = setInterval(async () => {
        const now = new Date();
        const timeRange = memoryCurrentEndTime - memoryCurrentStartTime;
        
        memoryCurrentEndTime = now;
        memoryCurrentStartTime = new Date(now.getTime() - timeRange);
        
        document.getElementById('memory-end-time').value = formatDateTimeLocal(memoryCurrentEndTime);
        document.getElementById('memory-start-time').value = formatDateTimeLocal(memoryCurrentStartTime);
        
        await updateMemoryChartIncremental();
    }, 1000);
}

// 内存增量更新图表数据
async function updateMemoryChartIncremental() {
    try {
        if (!memoryChart) {
            await loadMemoryChartData();
            return;
        }
        
        const endTime = formatDateTimeForQuery(memoryCurrentEndTime);
        const threeSecondsAgo = new Date(memoryCurrentEndTime.getTime() - 3000);
        const startTime = formatDateTimeForQuery(threeSecondsAgo);
        
        const url = `/api/history/memory?start=${encodeURIComponent(startTime)}&end=${encodeURIComponent(endTime)}`;
        const response = await fetch(url);
        
        if (!response.ok) return;
        
        const newData = await response.json();
        if (!newData || newData.length === 0) return;
        
        const lastLabel = memoryChart.data.labels[memoryChart.data.labels.length - 1];
        
        newData.forEach(item => {
            const timeLabel = item.timestamp.substring(11, 19);
            const value = item.usage;  // 数据库字段名为 usage
            
            if (timeLabel !== lastLabel && !memoryChart.data.labels.includes(timeLabel)) {
                memoryChart.data.labels.push(timeLabel);
                memoryChart.data.datasets[0].data.push(value);
            }
        });
        
        const timeRangeSeconds = (memoryCurrentEndTime - memoryCurrentStartTime) / 1000;
        const maxDataPoints = Math.ceil(timeRangeSeconds);
        
        while (memoryChart.data.labels.length > maxDataPoints) {
            memoryChart.data.labels.shift();
            memoryChart.data.datasets[0].data.shift();
        }
        
        const pointCount = memoryChart.data.labels.length;
        memoryChart.data.datasets[0].pointRadius = new Array(pointCount).fill(0);
        memoryChart.data.datasets[0].pointHoverRadius = new Array(pointCount).fill(5);
        memoryChart.data.datasets[0].pointHitRadius = new Array(pointCount).fill(10);
        
        memoryChart.update({
            duration: 800,
            easing: 'easeInOutQuart',
            lazy: false
        });
        
        document.getElementById('memory-data-count').textContent = memoryChart.data.labels.length;
        document.getElementById('memory-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        console.error('内存增量更新失败:', error);
    }
}

// 加载内存图表数据
async function loadMemoryChartData() {
    const startTime = formatDateTimeForQuery(memoryCurrentStartTime);
    const endTime = formatDateTimeForQuery(memoryCurrentEndTime);
    
    document.getElementById('memory-loading').style.display = 'block';
    document.getElementById('memory-error').style.display = 'none';
    document.getElementById('memory-chart-container').style.display = 'none';
    
    try {
        const url = `/api/history/memory?start=${encodeURIComponent(startTime)}&end=${encodeURIComponent(endTime)}`;
        const response = await fetch(url);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const memoryData = await response.json();
        
        if (!memoryData || memoryData.length === 0) {
            throw new Error('查询时间范围内无数据');
        }
        
        document.getElementById('memory-data-count').textContent = memoryData.length;
        
        renderMemoryChart(memoryData);
        
        document.getElementById('memory-loading').style.display = 'none';
        document.getElementById('memory-chart-container').style.display = 'block';
        
        document.getElementById('memory-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        document.getElementById('memory-loading').style.display = 'none';
        document.getElementById('memory-error').style.display = 'block';
        document.getElementById('memory-error').textContent = '加载数据失败: ' + error.message;
    }
}

// 渲染内存图表
function renderMemoryChart(data) {
    const ctx = document.getElementById('memory-chart');
    
    const labels = data.map(d => d.timestamp.substring(11, 19));
    const values = data.map(d => d.usage);  // 数据库字段名为 usage
    
    if (memoryChart) {
        memoryChart.destroy();
    }
    
    memoryChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: '内存使用率',
                data: values,
                borderColor: '#11cdef',
                backgroundColor: 'rgba(17, 205, 239, 0.1)',
                fill: true,
                tension: 0.4,
                borderWidth: 2,
                pointRadius: 0,
                pointHoverRadius: 5,
                pointHitRadius: 10,
                cubicInterpolationMode: 'monotone'
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: {
                duration: 800,
                easing: 'easeInOutQuart'
            },
            transitions: {
                active: {
                    animation: {
                        duration: 400
                    }
                }
            },
            plugins: {
                legend: {
                    display: false
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        label: function(context) {
                            return '内存使用率: ' + context.parsed.y.toFixed(2) + '%';
                        }
                    }
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    max: 100,
                    ticks: {
                        callback: function(value) {
                            return value + '%';
                        }
                    },
                    grid: {
                        color: 'rgba(0, 0, 0, 0.05)'
                    }
                },
                x: {
                    ticks: {
                        maxTicksLimit: 12,
                        maxRotation: 0,
                        minRotation: 0
                    },
                    grid: {
                        display: false
                    }
                }
            },
            interaction: {
                mode: 'nearest',
                axis: 'x',
                intersect: false
            }
        }
    });
}

// ==================== 磁盘 图表函数 ====================

// 初始化磁盘时间选择器（默认1分钟）
function initDiskTimeSelector() {
    const now = new Date();
    const oneMinuteAgo = new Date(now.getTime() - 60 * 1000);
    
    diskCurrentEndTime = now;
    diskCurrentStartTime = oneMinuteAgo;
    
    document.getElementById('disk-end-time').value = formatDateTimeLocal(now);
    document.getElementById('disk-start-time').value = formatDateTimeLocal(oneMinuteAgo);
}

// 磁盘快速选择时间范围
function diskSelectQuickTime(minutes) {
    document.querySelectorAll('#disk-chart-section .quick-time-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    event.target.classList.add('active');
    
    const now = new Date();
    const past = new Date(now.getTime() - minutes * 60 * 1000);
    
    diskCurrentEndTime = now;
    diskCurrentStartTime = past;
    
    document.getElementById('disk-end-time').value = formatDateTimeLocal(now);
    document.getElementById('disk-start-time').value = formatDateTimeLocal(past);
    
    loadDiskChartData();
}

// 磁盘手动查询（停止自动刷新）
function diskManualQuery() {
    if (diskAutoRefreshTimer) {
        clearInterval(diskAutoRefreshTimer);
        diskAutoRefreshTimer = null;
    }
    
    const startInput = document.getElementById('disk-start-time').value;
    const endInput = document.getElementById('disk-end-time').value;
    
    diskCurrentStartTime = new Date(startInput);
    diskCurrentEndTime = new Date(endInput);
    
    loadDiskChartData();
}

// 启动磁盘自动刷新
function startDiskAutoRefresh() {
    if (diskAutoRefreshTimer) {
        clearInterval(diskAutoRefreshTimer);
    }
    
    diskAutoRefreshTimer = setInterval(async () => {
        const now = new Date();
        const timeRange = diskCurrentEndTime - diskCurrentStartTime;
        
        diskCurrentEndTime = now;
        diskCurrentStartTime = new Date(now.getTime() - timeRange);
        
        document.getElementById('disk-end-time').value = formatDateTimeLocal(diskCurrentEndTime);
        document.getElementById('disk-start-time').value = formatDateTimeLocal(diskCurrentStartTime);
        
        await updateDiskChartIncremental();
    }, 1000);
}

// 磁盘增量更新图表数据
async function updateDiskChartIncremental() {
    try {
        if (!diskChart) {
            await loadDiskChartData();
            return;
        }
        
        const endTime = formatDateTimeForQuery(diskCurrentEndTime);
        const threeSecondsAgo = new Date(diskCurrentEndTime.getTime() - 3000);
        const startTime = formatDateTimeForQuery(threeSecondsAgo);
        
        const url = `/api/history/disk?start=${encodeURIComponent(startTime)}&end=${encodeURIComponent(endTime)}`;
        const response = await fetch(url);
        
        if (!response.ok) return;
        
        const newData = await response.json();
        if (!newData || newData.length === 0) return;
        
        const lastLabel = diskChart.data.labels[diskChart.data.labels.length - 1];
        
        // 获取当前图表使用的单位（从 Y1 轴标题中提取）
        const currentUnitText = diskChart.options.scales.y1.title.text;
        const currentUnit = currentUnitText.match(/\((.+?)\)/)[1];  // 提取括号中的单位
        
        // 计算当前单位的 divider
        const unitMap = { 'KB/s': 1, 'MB/s': 1024, 'GB/s': 1024*1024, 'TB/s': 1024*1024*1024 };
        const currentDivider = unitMap[currentUnit] || 1;
        
        newData.forEach(item => {
            const timeLabel = item.timestamp.substring(11, 19);
            const usageValue = item.usage;
            const readSpeedValue = (item.read_speed / 1024) / currentDivider;  // KB/s -> 当前单位
            const writeSpeedValue = (item.write_speed / 1024) / currentDivider;  // KB/s -> 当前单位
            
            if (timeLabel !== lastLabel && !diskChart.data.labels.includes(timeLabel)) {
                diskChart.data.labels.push(timeLabel);
                diskChart.data.datasets[0].data.push(usageValue);  // 使用率
                diskChart.data.datasets[1].data.push(readSpeedValue);  // 读速度
                diskChart.data.datasets[2].data.push(writeSpeedValue);  // 写速度
            }
        });
        
        const timeRangeSeconds = (diskCurrentEndTime - diskCurrentStartTime) / 1000;
        const maxDataPoints = Math.ceil(timeRangeSeconds);
        
        while (diskChart.data.labels.length > maxDataPoints) {
            diskChart.data.labels.shift();
            diskChart.data.datasets[0].data.shift();  // 使用率
            diskChart.data.datasets[1].data.shift();  // 读速度
            diskChart.data.datasets[2].data.shift();  // 写速度
        }
        
        const pointCount = diskChart.data.labels.length;
        // 为所有数据集设置 pointRadius
        for (let i = 0; i < diskChart.data.datasets.length; i++) {
            diskChart.data.datasets[i].pointRadius = new Array(pointCount).fill(0);
            diskChart.data.datasets[i].pointHoverRadius = new Array(pointCount).fill(5);
            diskChart.data.datasets[i].pointHitRadius = new Array(pointCount).fill(10);
        }
        
        diskChart.update({
            duration: 800,
            easing: 'easeInOutQuart',
            lazy: false
        });
        
        document.getElementById('disk-data-count').textContent = diskChart.data.labels.length;
        document.getElementById('disk-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        console.error('磁盘增量更新失败:', error);
    }
}

// 加载磁盘图表数据
async function loadDiskChartData() {
    const startTime = formatDateTimeForQuery(diskCurrentStartTime);
    const endTime = formatDateTimeForQuery(diskCurrentEndTime);
    
    document.getElementById('disk-loading').style.display = 'block';
    document.getElementById('disk-error').style.display = 'none';
    document.getElementById('disk-chart-container').style.display = 'none';
    
    try {
        const url = `/api/history/disk?start=${encodeURIComponent(startTime)}&end=${encodeURIComponent(endTime)}`;
        const response = await fetch(url);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const diskData = await response.json();
        
        if (!diskData || diskData.length === 0) {
            throw new Error('查询时间范围内无数据');
        }
        
        document.getElementById('disk-data-count').textContent = diskData.length;
        
        renderDiskChart(diskData);
        
        document.getElementById('disk-loading').style.display = 'none';
        document.getElementById('disk-chart-container').style.display = 'block';
        
        document.getElementById('disk-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        document.getElementById('disk-loading').style.display = 'none';
        document.getElementById('disk-error').style.display = 'block';
        document.getElementById('disk-error').textContent = '加载数据失败: ' + error.message;
    }
}

// 渲染磁盘图表
function renderDiskChart(data) {
    const ctx = document.getElementById('disk-chart');
    
    const labels = data.map(d => d.timestamp.substring(11, 19));
    const usageValues = data.map(d => d.usage);  // 使用率百分比
    const readSpeedValues = data.map(d => d.read_speed / 1024);  // 读速度 KB/s
    const writeSpeedValues = data.map(d => d.write_speed / 1024);  // 写速度 KB/s
    
    // 计算最大值以确定最佳单位
    const maxSpeed = Math.max(...readSpeedValues, ...writeSpeedValues);
    const speedUnit = getBestUnit(maxSpeed);
    
    if (diskChart) {
        diskChart.destroy();
    }
    
    diskChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                {
                    label: '磁盘使用率',
                    data: usageValues,
                    borderColor: '#fb6340',
                    backgroundColor: 'rgba(251, 99, 64, 0.1)',
                    fill: true,
                    tension: 0.4,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 5,
                    pointHitRadius: 10,
                    cubicInterpolationMode: 'monotone',
                    yAxisID: 'y'
                },
                {
                    label: '读取速度',
                    data: readSpeedValues.map(v => v / speedUnit.divider),  // 转换为目标单位
                    borderColor: '#5e72e4',  // 蓝色
                    backgroundColor: 'transparent',  // 不填充颜色
                    fill: false,
                    tension: 0.4,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 5,
                    pointHitRadius: 10,
                    cubicInterpolationMode: 'monotone',
                    yAxisID: 'y1'
                },
                {
                    label: '写入速度',
                    data: writeSpeedValues.map(v => v / speedUnit.divider),  // 转换为目标单位
                    borderColor: '#2dce89',  // 绿色
                    backgroundColor: 'transparent',  // 不填充颜色
                    fill: false,
                    tension: 0.4,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 5,
                    pointHitRadius: 10,
                    cubicInterpolationMode: 'monotone',
                    yAxisID: 'y1'
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: {
                duration: 800,
                easing: 'easeInOutQuart'
            },
            transitions: {
                active: {
                    animation: {
                        duration: 400
                    }
                }
            },
            plugins: {
                legend: {
                    display: true,
                    position: 'top',
                    labels: {
                        usePointStyle: true,
                        padding: 15,
                        font: {
                            size: 12
                        }
                    }
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        label: function(context) {
                            const label = context.dataset.label || '';
                            const value = context.parsed.y;
                            if (label === '磁盘使用率') {
                                return label + ': ' + value.toFixed(2) + '%';
                            } else {
                                // 显示实际值（KB）转换后的单位
                                const actualValue = value * speedUnit.divider;
                                return label + ': ' + formatBytes(actualValue);
                            }
                        }
                    }
                }
            },
            scales: {
                y: {
                    type: 'linear',
                    display: true,
                    position: 'left',
                    beginAtZero: true,
                    max: 100,
                    title: {
                        display: true,
                        text: '使用率 (%)'
                    },
                    ticks: {
                        callback: function(value) {
                            return value + '%';
                        }
                    },
                    grid: {
                        color: 'rgba(0, 0, 0, 0.05)'
                    }
                },
                y1: {
                    type: 'linear',
                    display: true,
                    position: 'right',
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: '读写速度 (' + speedUnit.unit + ')'
                    },
                    ticks: {
                        callback: function(value) {
                            return value.toFixed(1) + ' ' + speedUnit.unit;
                        }
                    },
                    grid: {
                        drawOnChartArea: false  // 不显示右侧网格线
                    }
                },
                x: {
                    ticks: {
                        maxTicksLimit: 12,
                        maxRotation: 0,
                        minRotation: 0
                    },
                    grid: {
                        display: false
                    }
                }
            },
            interaction: {
                mode: 'nearest',
                axis: 'x',
                intersect: false
            }
        }
    });
}

// ==================== 网络 图表相关 ====================

let networkChart = null;
let networkCurrentStartTime = null;
let networkCurrentEndTime = null;
let networkAutoRefreshTimer = null;

// ==================== 网络 图表函数 ====================

// 初始化网络时间选择器
function initNetworkTimeSelector() {
    const now = new Date();
    const oneMinuteAgo = new Date(now.getTime() - 60 * 1000);
    
    networkCurrentStartTime = oneMinuteAgo;
    networkCurrentEndTime = now;
    
    document.getElementById('network-start-time').value = formatDateTimeLocal(oneMinuteAgo);
    document.getElementById('network-end-time').value = formatDateTimeLocal(now);
}

// 网络快捷时间选择
function networkSelectQuickTime(minutes) {
    // 移除所有按钮的 active 状态
    document.querySelectorAll('#network-chart-section .quick-time-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    
    // 添加当前按钮的 active 状态
    if (event && event.target) {
        event.target.classList.add('active');
    }
    
    const now = new Date();
    const start = new Date(now.getTime() - minutes * 60 * 1000);
    
    networkCurrentStartTime = start;
    networkCurrentEndTime = now;
    
    document.getElementById('network-start-time').value = formatDateTimeLocal(start);
    document.getElementById('network-end-time').value = formatDateTimeLocal(now);
    
    loadNetworkChartData();
}

// 网络手动查询
function networkManualQuery() {
    const startTime = document.getElementById('network-start-time').value;
    const endTime = document.getElementById('network-end-time').value;
    
    if (!startTime || !endTime) {
        alert('请选择开始和结束时间');
        return;
    }
    
    networkCurrentStartTime = new Date(startTime);
    networkCurrentEndTime = new Date(endTime);
    
    // 移除所有快捷按钮的 active 状态
    document.querySelectorAll('#network-chart-section .quick-time-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    
    loadNetworkChartData();
}

// 加载网络图表数据
async function loadNetworkChartData() {
    try {
        document.getElementById('network-loading').style.display = 'block';
        document.getElementById('network-error').style.display = 'none';
        document.getElementById('network-chart-container').style.display = 'none';
        
        const startStr = formatDateTimeForQuery(networkCurrentStartTime);
        const endStr = formatDateTimeForQuery(networkCurrentEndTime);
        
        const response = await fetch(`/api/history/network?start=${encodeURIComponent(startStr)}&end=${encodeURIComponent(endStr)}`);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const networkData = await response.json();
        
        if (!networkData || networkData.length === 0) {
            throw new Error('查询时间范围内无数据');
        }
        
        document.getElementById('network-data-count').textContent = networkData.length;
        
        renderNetworkChart(networkData);
        
        document.getElementById('network-loading').style.display = 'none';
        document.getElementById('network-chart-container').style.display = 'block';
        
        document.getElementById('network-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        document.getElementById('network-loading').style.display = 'none';
        document.getElementById('network-error').style.display = 'block';
        document.getElementById('network-error').textContent = '加载数据失败: ' + error.message;
    }
}

// 渲染网络图表
function renderNetworkChart(data) {
    const ctx = document.getElementById('network-chart');
    
    const labels = data.map(d => d.timestamp.substring(11, 19));
    const uploadValues = data.map(d => d.upload_speed / 1024);  // 转换为 KB/s
    const downloadValues = data.map(d => d.download_speed / 1024);  // 转换为 KB/s
    
    // 计算最大值以确定最佳单位
    const maxSpeed = Math.max(...uploadValues, ...downloadValues);
    const speedUnit = getBestUnit(maxSpeed);
    
    if (networkChart) {
        networkChart.destroy();
    }
    
    networkChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [
                {
                    label: '上行速度',
                    data: uploadValues.map(v => v / speedUnit.divider),  // 转换为目标单位
                    borderColor: '#5e72e4',
                    backgroundColor: 'rgba(94, 114, 228, 0.1)',
                    fill: true,
                    tension: 0.4,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 5,
                    pointHitRadius: 10,
                    cubicInterpolationMode: 'monotone'
                },
                {
                    label: '下行速度',
                    data: downloadValues.map(v => v / speedUnit.divider),  // 转换为目标单位
                    borderColor: '#2dce89',
                    backgroundColor: 'rgba(45, 206, 137, 0.1)',
                    fill: true,
                    tension: 0.4,
                    borderWidth: 2,
                    pointRadius: 0,
                    pointHoverRadius: 5,
                    pointHitRadius: 10,
                    cubicInterpolationMode: 'monotone'
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: {
                duration: 800,
                easing: 'easeInOutQuart'
            },
            transitions: {
                active: {
                    animation: {
                        duration: 400
                    }
                }
            },
            plugins: {
                legend: {
                    display: true,
                    position: 'top',
                    labels: {
                        usePointStyle: true,
                        padding: 15,
                        font: {
                            size: 12
                        }
                    }
                },
                tooltip: {
                    mode: 'index',
                    intersect: false,
                    callbacks: {
                        label: function(context) {
                            const label = context.dataset.label || '';
                            const value = context.parsed.y;
                            // 显示实际值（KB）转换后的单位
                            const actualValue = value * speedUnit.divider;
                            return label + ': ' + formatBytes(actualValue);
                        }
                    }
                }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: '速度 (' + speedUnit.unit + ')'
                    },
                    ticks: {
                        callback: function(value) {
                            return value.toFixed(1) + ' ' + speedUnit.unit;
                        }
                    },
                    grid: {
                        color: 'rgba(0, 0, 0, 0.05)'
                    }
                },
                x: {
                    ticks: {
                        maxTicksLimit: 12,
                        maxRotation: 0,
                        minRotation: 0
                    },
                    grid: {
                        display: false
                    }
                }
            },
            interaction: {
                mode: 'nearest',
                axis: 'x',
                intersect: false
            }
        }
    });
}

// 增量更新网络图表
async function updateNetworkChartIncremental() {
    try {
        // 查询最近3秒的数据
        const now = new Date();
        const threeSecondsAgo = new Date(now.getTime() - 3000);
        const startStr = formatDateTimeForQuery(threeSecondsAgo);
        const endStr = formatDateTimeForQuery(now);
        
        const response = await fetch(`/api/history/network?start=${encodeURIComponent(startStr)}&end=${encodeURIComponent(endStr)}`);
        
        if (!response.ok) return;
        
        const newData = await response.json();
        if (!newData || newData.length === 0) return;
        
        // 计算最大数据点数
        const timeRangeMinutes = (networkCurrentEndTime - networkCurrentStartTime) / 1000 / 60;
        const maxDataPoints = Math.ceil(timeRangeMinutes * 60); // 每秒60个数据点
        
        // 获取最后一个数据点的时间戳，用于去重
        const lastLabel = networkChart.data.labels[networkChart.data.labels.length - 1];
        
        // 获取当前图表使用的单位
        const currentUnitText = networkChart.options.scales.y.title.text;
        const currentUnit = currentUnitText.match(/\((.+?)\)/)[1];  // 提取括号中的单位
        
        // 计算当前单位的 divider
        const unitMap = { 'KB/s': 1, 'MB/s': 1024, 'GB/s': 1024*1024, 'TB/s': 1024*1024*1024 };
        const currentDivider = unitMap[currentUnit] || 1;
        
        // 添加新数据（反转以保证时间顺序）
        newData.forEach(item => {
            const timeLabel = item.timestamp.substring(11, 19);
            const uploadValue = (item.upload_speed / 1024) / currentDivider;  // KB/s -> 当前单位
            const downloadValue = (item.download_speed / 1024) / currentDivider;  // KB/s -> 当前单位
            
            // 去重：如果该时间点已经存在，则不添加
            if (timeLabel !== lastLabel && !networkChart.data.labels.includes(timeLabel)) {
                networkChart.data.labels.push(timeLabel);
                networkChart.data.datasets[0].data.push(uploadValue);  // 上行速度
                networkChart.data.datasets[1].data.push(downloadValue);  // 下行速度
            }
        });
        
        // 删除超出时间窗口的旧数据
        while (networkChart.data.labels.length > maxDataPoints) {
            networkChart.data.labels.shift();
            networkChart.data.datasets[0].data.shift();
            networkChart.data.datasets[1].data.shift();
        }
        
        // 更新图表（带平滑动画）
        networkChart.update({
            duration: 800,
            easing: 'easeInOutQuart',
            lazy: false
        });
        
        // 更新数据总量和时间显示
        document.getElementById('network-data-count').textContent = networkChart.data.labels.length;
        document.getElementById('network-update-time').textContent = new Date().toLocaleString('zh-CN');
        
    } catch (error) {
        console.error('增量更新网络图表失败:', error);
    }
}

// 开启网络图表自动刷新
function startNetworkAutoRefresh() {
    // 清除旧的定时器
    if (networkAutoRefreshTimer) {
        clearInterval(networkAutoRefreshTimer);
    }
    
    // 每秒更新时间窗口并增量更新
    networkAutoRefreshTimer = setInterval(async () => {
        const now = new Date();
        const timeRange = networkCurrentEndTime - networkCurrentStartTime;
        
        networkCurrentEndTime = now;
        networkCurrentStartTime = new Date(now.getTime() - timeRange);
        
        document.getElementById('network-end-time').value = formatDateTimeLocal(networkCurrentEndTime);
        document.getElementById('network-start-time').value = formatDateTimeLocal(networkCurrentStartTime);
        
        await updateNetworkChartIncremental();
    }, 1000);
}

// ==================== 页面初始化 ====================

// 获取数据时间范围并设置日期选择框限制
async function loadDataTimeRange() {
    try {
        const response = await fetch('/api/stats/timerange');
        if (!response.ok) return;
        
        const data = await response.json();
        if (!data.min_time || !data.max_time) return;
        
        // 转换时间格式："2025-12-30 17:04:45" -> "2025-12-30T17:04"
        const minTime = data.min_time.replace(' ', 'T').substring(0, 16);
        const maxTime = data.max_time.replace(' ', 'T').substring(0, 16);
        
        // 设置所有日期选择框的 min/max 属性
        const timeInputs = [
            'cpu-start-time', 'cpu-end-time',
            'memory-start-time', 'memory-end-time',
            'disk-start-time', 'disk-end-time',
            'network-start-time', 'network-end-time'
        ];
        
        timeInputs.forEach(id => {
            const input = document.getElementById(id);
            if (input) {
                input.min = minTime;
                input.max = maxTime;
            }
        });
        
        console.log(`数据时间范围: ${minTime} ~ ${maxTime}`);
    } catch (error) {
        console.error('获取数据时间范围失败:', error);
    }
}

window.addEventListener('load', function() {
    // 获取数据时间范围并设置日期选择框限制
    loadDataTimeRange();
    
    // 初始化CPU图表
    initCPUTimeSelector();
    loadCPUChartData();
    startCPUAutoRefresh();
    
    // 初始化内存图表
    initMemoryTimeSelector();
    loadMemoryChartData();
    startMemoryAutoRefresh();
    
    // 初始化磁盘图表
    initDiskTimeSelector();
    loadDiskChartData();
    startDiskAutoRefresh();
    
    // 初始化网络图表
    initNetworkTimeSelector();
    loadNetworkChartData();
    startNetworkAutoRefresh();
});
