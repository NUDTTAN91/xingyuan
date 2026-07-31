/**
 * 星垣 - 远程主机监控 JavaScript
 * Author: tan91
 */

// ==================== 全局变量 ====================

let currentHostId = null;  // 当前选中的主机 ID
let currentTab = 'metrics'; // 当前选中的 Tab
let refreshInterval = null; // 自动刷新定时器
let statusCheckInterval = null; // 状态检查定时器
let metricsRefreshInterval = null; // 缩略图监控数据刷新定时器
let hostMetricsCache = {}; // 主机监控数据缓存

// ==================== 页面初始化 ====================

window.addEventListener('load', () => {
    console.log('远程主机监控页面加载完成');
    
    // 加载主机列表
    loadHostList();
    
    // 启动状态检查定时器（每 10 秒检查一次）
    statusCheckInterval = setInterval(checkAllHostsStatus, 10000);
    
    // 缩略图监控数据刷新（每 5 秒一次：每次会对所有主机各发一个代理请求，
    // 频率过高会对本机和所有远程主机造成请求风暴）
    metricsRefreshInterval = setInterval(updateAllHostsMetrics, 5000);
    updateAllHostsMetrics();
    
    // 更新统计数据
    updateStats();
    
    // 更新时间
    setInterval(() => {
        document.getElementById('update-time').textContent = new Date().toLocaleTimeString('zh-CN');
    }, 1000);
});

// 页面关闭时清理所有定时器
window.addEventListener('beforeunload', () => {
    if (statusCheckInterval) clearInterval(statusCheckInterval);
    if (metricsRefreshInterval) clearInterval(metricsRefreshInterval);
    if (refreshInterval) clearInterval(refreshInterval);
});

// ==================== 主机列表管理 ====================

// 加载主机列表
async function loadHostList() {
    try {
        const response = await fetch('/api/remote/hosts');
        const result = await response.json();
        
        if (!result.success) {
            console.error('加载主机列表失败:', result.message);
            return;
        }
        
        const hosts = result.data || [];
        renderHostList(hosts);
        
        // 如果有主机，默认选中第一个
        if (hosts.length > 0 && !currentHostId) {
            selectHostById(hosts[0].id);
        }
        
        // 更新统计
        updateStats();
        
    } catch (error) {
        console.error('加载主机列表异常:', error);
    }
}

// 渲染主机列表
function renderHostList(hosts) {
    const hostList = document.getElementById('host-list');
    
    if (!hosts || hosts.length === 0) {
        hostList.innerHTML = `
            <div style="padding: 20px; text-align: center; color: #8898aa;">
                <div style="font-size: 48px; margin-bottom: 10px;">📡</div>
                <div>暂无远程主机</div>
                <div style="font-size: 12px; margin-top: 5px;">点击上方按钮添加主机</div>
            </div>
        `;
        return;
    }
    
    hostList.innerHTML = hosts.map(host => {
        const isActive = host.id === currentHostId;
        return `
            <div class="host-item ${isActive ? 'active' : ''}" 
                 onclick="selectHostById(${host.id})"
                 data-host-id="${host.id}">
                <div class="host-item-header">
                    <div class="host-name">
                        <span class="host-status offline" id="status-${host.id}"></span>
                        ${host.name}
                    </div>
                    <div style="display: flex; gap: 4px;">
                        <button class="icon-btn" onclick="openHostLink(${host.id}, event)" title="打开主机监控">
                            🔗
                        </button>
                        <button class="icon-btn" onclick="editHost(${host.id}, event)" title="编辑主机">
                            ✏️
                        </button>
                        <button class="icon-btn" onclick="deleteHost(${host.id}, event)" title="删除主机">
                            🗑️
                        </button>
                    </div>
                </div>
                
                <!-- IP地址和监控数据在同一行 -->
                <div style="display: flex; justify-content: space-between; align-items: center; gap: 10px;">
                    <div class="host-address" style="flex-shrink: 0;">${host.address}:${host.port}</div>
                    
                    <!-- 紧凑的监控数据 -->
                    <div id="metrics-${host.id}" style="flex: 1; min-width: 0; font-size: 11px;">
                        <div style="display: flex; align-items: center; gap: 4px; margin-bottom: 3px;">
                            <span style="color: #5e72e4; font-size: 10px;">🖥️</span>
                            <div style="flex: 1; height: 3px; background: #e9ecef; border-radius: 2px; overflow: hidden;">
                                <div id="cpu-bar-${host.id}" style="height: 100%; background: linear-gradient(90deg, #5e72e4, #825ee4); width: 0%; transition: width 0.3s;"></div>
                            </div>
                            <span id="cpu-${host.id}" style="font-weight: 600; color: #5e72e4; font-size: 10px; min-width: 32px; text-align: right;">--%</span>
                        </div>
                        <div style="display: flex; align-items: center; gap: 4px;">
                            <span style="color: #11cdef; font-size: 10px;">💾</span>
                            <div style="flex: 1; height: 3px; background: #e9ecef; border-radius: 2px; overflow: hidden;">
                                <div id="mem-bar-${host.id}" style="height: 100%; background: linear-gradient(90deg, #11cdef, #1171ef); width: 0%; transition: width 0.3s;"></div>
                            </div>
                            <span id="mem-${host.id}" style="font-weight: 600; color: #11cdef; font-size: 10px; min-width: 32px; text-align: right;">--%</span>
                        </div>
                    </div>
                </div>
                
                <div class="host-info" id="info-${host.id}">检查中...</div>
            </div>
        `;
    }).join('');
    
    // 立即检查状态和监控数据
    checkAllHostsStatus();
    updateAllHostsMetrics();
}

// 选择主机（通过元素）
function selectHost(element, hostId) {
    selectHostById(hostId);
}

// 选择主机（通过 ID）
async function selectHostById(hostId) {
    // 移除所有 active 状态
    document.querySelectorAll('.host-item').forEach(item => {
        item.classList.remove('active');
    });
    
    // 添加当前 active 状态
    const element = document.querySelector(`[data-host-id="${hostId}"]`);
    if (element) {
        element.classList.add('active');
    }
    
    currentHostId = hostId;
    
    // 获取主机信息
    try {
        const response = await fetch(`/api/remote/hosts`);
        const result = await response.json();
        
        if (result.success) {
            const host = result.data.find(h => h.id === hostId);
            if (host) {
                updateMonitorHeader(host);
            }
        }
    } catch (error) {
        console.error('获取主机信息失败:', error);
    }
    
    // 加载监控数据
    loadMonitorData();
}

// 更新监控区域头部
function updateMonitorHeader(host) {
    const monitorTitle = document.querySelector('.monitor-title');
    const monitorSubtitle = document.querySelector('.monitor-subtitle');
    
    if (monitorTitle) {
        monitorTitle.textContent = `📡 ${host.name}`;
    }
    
    if (monitorSubtitle) {
        monitorSubtitle.textContent = `${host.address}:${host.port} · 在线`;
    }
}

// ==================== 主机状态检查和监控数据更新 ====================

// 更新所有主机的监控数据
async function updateAllHostsMetrics() {
    try {
        const response = await fetch('/api/remote/hosts');
        const result = await response.json();
        
        if (!result.success || !result.data) {
            return;
        }
        
        const hosts = result.data;
        
        // 并发获取所有在线主机的监控数据
        const promises = hosts.map(async (host) => {
            try {
                const metricsResponse = await fetch(`/api/remote/${host.id}/metrics`);
                if (metricsResponse.ok) {
                    const metrics = await metricsResponse.json();
                    updateHostMetricsDisplay(host.id, metrics);
                    // 更新主机状态显示（实时更新最后更新时间）
                    const statusElement = document.getElementById(`status-${host.id}`);
                    if (statusElement) {
                        // 获取当前主机的状态信息
                        const currentStatus = {
                            online: true,
                            last_check: new Date().toISOString()
                        };
                        updateHostStatus(host.id, currentStatus);
                    }
                    // 缓存数据
                    hostMetricsCache[host.id] = metrics;
                    
                    // 如果是当前选中的主机，同时更新右侧详细数据
                    if (host.id === currentHostId && currentTab === 'metrics') {
                        renderMetrics(metrics);
                    }
                } else {
                    // 获取失败，清空显示
                    updateHostMetricsDisplay(host.id, null);
                    // 更新主机状态为离线
                    const statusElement = document.getElementById(`status-${host.id}`);
                    if (statusElement) {
                        const offlineStatus = {
                            online: false,
                            error: '无法连接'
                        };
                        updateHostStatus(host.id, offlineStatus);
                    }
                }
            } catch (error) {
                // 获取失败，清空显示
                updateHostMetricsDisplay(host.id, null);
                // 更新主机状态为离线
                const statusElement = document.getElementById(`status-${host.id}`);
                if (statusElement) {
                    const offlineStatus = {
                        online: false,
                        error: error.message
                    };
                    updateHostStatus(host.id, offlineStatus);
                }
            }
        });
        
        await Promise.all(promises);
        
    } catch (error) {
        console.error('更新主机监控数据异常:', error);
    }
}

// 更新主机卡片的监控数据显示
function updateHostMetricsDisplay(hostId, metrics) {
    const cpuText = document.getElementById(`cpu-${hostId}`);
    const cpuBar = document.getElementById(`cpu-bar-${hostId}`);
    const memText = document.getElementById(`mem-${hostId}`);
    const memBar = document.getElementById(`mem-bar-${hostId}`);
    
    if (!cpuText || !cpuBar || !memText || !memBar) {
        return;
    }
    
    if (!metrics || !metrics.cpu || !metrics.memory) {
        // 数据获取失败，显示占位符
        cpuText.textContent = '--%';
        cpuBar.style.width = '0%';
        memText.textContent = '--%';
        memBar.style.width = '0%';
        return;
    }
    
    const cpuUsage = metrics.cpu.usage_percent || 0;
    const memUsage = metrics.memory.usage_percent || 0;
    
    cpuText.textContent = cpuUsage.toFixed(1) + '%';
    cpuBar.style.width = cpuUsage + '%';
    
    memText.textContent = memUsage.toFixed(1) + '%';
    memBar.style.width = memUsage + '%';
}

// 检查所有主机状态
async function checkAllHostsStatus() {
    try {
        const response = await fetch('/api/remote/hosts/status/all');
        const result = await response.json();
        
        if (!result.success) {
            return;
        }
        
        const statuses = result.data || {};
        
        // 更新每个主机的状态显示
        Object.keys(statuses).forEach(hostId => {
            const status = statuses[hostId];
            updateHostStatus(parseInt(hostId), status);
        });
        
    } catch (error) {
        console.error('检查主机状态异常:', error);
    }
}

// 更新主机状态显示
function updateHostStatus(hostId, status) {
    const statusDot = document.getElementById(`status-${hostId}`);
    const infoText = document.getElementById(`info-${hostId}`);
    
    if (!statusDot || !infoText) return;
    
    if (status.online) {
        statusDot.className = 'host-status online';
        if (status.error) {
            // 在线但认证失败（用户名或密码错误）
            infoText.textContent = status.error;
        } else {
            // 使用当前时间作为最后更新时间
            const lastCheckTime = formatDateTime(new Date());
            infoText.textContent = `最后更新: ${lastCheckTime}`;
        }
    } else {
        statusDot.className = 'host-status offline';
        infoText.textContent = `离线 (${status.error || '无法连接'})`;
    }
}

// 格式化日期时间为 YYYY-MM-DD HH:mm:ss
function formatDateTime(date) {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, '0');
    const day = String(date.getDate()).padStart(2, '0');
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    const seconds = String(date.getSeconds()).padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
}

// ==================== 监控数据加载 ====================

// 切换监控 Tab
function switchMonitorTab(element, tabName) {
    // 移除所有 active 状态
    document.querySelectorAll('.monitor-tab').forEach(tab => {
        tab.classList.remove('active');
    });
    
    // 添加当前 active 状态
    element.classList.add('active');
    
    currentTab = tabName;
    
    // 停止自动刷新
    if (refreshInterval) {
        clearInterval(refreshInterval);
        refreshInterval = null;
    }
    
    // 加载对应的监控数据
    loadMonitorData();
}

// 加载监控数据
async function loadMonitorData() {
    if (!currentHostId) {
        showEmptyState('请先选择一个远程主机');
        return;
    }
    
    const content = document.getElementById('monitor-content');
    
    if (currentTab === 'metrics') {
        await loadRemoteMetrics();
        // 启动自动刷新（1秒）
        if (refreshInterval) clearInterval(refreshInterval);
        refreshInterval = setInterval(loadRemoteMetrics, 1000);
    } else if (currentTab === 'docker') {
        await loadRemoteDocker();
    } else if (currentTab === 'history') {
        showEmptyState('📈', '历史图表', '远程主机的历史监控数据（功能开发中）');
    }
}

// 加载远程主机的实时监控数据
async function loadRemoteMetrics() {
    try {
        const response = await fetch(`/api/remote/${currentHostId}/metrics`);
        
        if (!response.ok) {
            if (response.status === 401) {
                throw new Error('认证失败，请检查用户名和密码');
            } else if (response.status === 404) {
                throw new Error('远程主机不存在或服务未启动');
            } else if (response.status === 500) {
                throw new Error('远程主机内部错误');
            } else {
                throw new Error(`HTTP ${response.status}`);
            }
        }
        
        const metrics = await response.json();
        
        // 调试：打印返回的数据
        console.log('远程主机返回的数据:', metrics);
        console.log('CPU数据:', metrics.cpu);
        console.log('Memory数据:', metrics.memory);
        console.log('Disk数据:', metrics.disk);
        console.log('Network数据:', metrics.network);
        
        // 验证数据结构
        if (!metrics || typeof metrics !== 'object') {
            throw new Error('远程主机返回的数据格式错误');
        }
        
        renderMetrics(metrics);
        
    } catch (error) {
        console.error('加载远程监控数据失败:', error);
        console.error('错误堆栈:', error.stack);
        
        // 停止自动刷新
        if (refreshInterval) {
            clearInterval(refreshInterval);
            refreshInterval = null;
        }
        
        showError('无法加载监控数据: ' + error.message);
    }
}

// 渲染监控数据
function renderMetrics(metrics) {
    const content = document.getElementById('monitor-content');
    
    // 数据校验和默认值（使用与后端一致的字段名）
    const safeMetrics = {
        cpu: {
            usage_percent: (metrics.cpu && typeof metrics.cpu.usage_percent === 'number') ? metrics.cpu.usage_percent : 0,
            cores: (metrics.cpu && metrics.cpu.cores) ? metrics.cpu.cores : 0
        },
        memory: {
            usage_percent: (metrics.memory && typeof metrics.memory.usage_percent === 'number') ? metrics.memory.usage_percent : 0,
            used: (metrics.memory && typeof metrics.memory.used === 'number') ? metrics.memory.used : 0,
            total: (metrics.memory && typeof metrics.memory.total === 'number') ? metrics.memory.total : 0,
            free: (metrics.memory && typeof metrics.memory.free === 'number') ? metrics.memory.free : 0
        },
        disk: {
            usage_percent: (metrics.disk && typeof metrics.disk.usage_percent === 'number') ? metrics.disk.usage_percent : 0,
            used: (metrics.disk && typeof metrics.disk.used === 'number') ? metrics.disk.used : 0,
            total: (metrics.disk && typeof metrics.disk.total === 'number') ? metrics.disk.total : 0,
            free: (metrics.disk && typeof metrics.disk.free === 'number') ? metrics.disk.free : 0,
            read_speed: (metrics.disk && typeof metrics.disk.read_speed === 'number') ? metrics.disk.read_speed : 0,
            write_speed: (metrics.disk && typeof metrics.disk.write_speed === 'number') ? metrics.disk.write_speed : 0
        },
        network: {
            upload_speed: (metrics.network && typeof metrics.network.upload_speed === 'number') ? metrics.network.upload_speed : 0,
            download_speed: (metrics.network && typeof metrics.network.download_speed === 'number') ? metrics.network.download_speed : 0,
            bytes_sent: (metrics.network && typeof metrics.network.bytes_sent === 'number') ? metrics.network.bytes_sent : 0,
            bytes_recv: (metrics.network && typeof metrics.network.bytes_recv === 'number') ? metrics.network.bytes_recv : 0
        }
    };
    
    content.innerHTML = `
        <div class="metrics-grid">
            <!-- CPU -->
            <div class="metric-card">
                <div class="metric-header">
                    <span class="metric-title">CPU使用率</span>
                    <div class="metric-icon cpu">🖥️</div>
                </div>
                <div class="metric-value">${safeMetrics.cpu.usage_percent.toFixed(1)}%</div>
                <div class="metric-bar">
                    <div class="metric-bar-fill bar-cpu" style="width: ${safeMetrics.cpu.usage_percent}%"></div>
                </div>
                <div class="metric-detail">
                    <span>${safeMetrics.cpu.cores} 核心</span>
                    <span>空闲: ${(100 - safeMetrics.cpu.usage_percent).toFixed(1)}%</span>
                </div>
            </div>

            <!-- 内存 -->
            <div class="metric-card">
                <div class="metric-header">
                    <span class="metric-title">内存使用率</span>
                    <div class="metric-icon memory">💾</div>
                </div>
                <div class="metric-value">${safeMetrics.memory.usage_percent.toFixed(1)}%</div>
                <div class="metric-bar">
                    <div class="metric-bar-fill bar-memory" style="width: ${safeMetrics.memory.usage_percent}%"></div>
                </div>
                <div class="metric-detail">
                    <span>${formatBytes(safeMetrics.memory.used)} / ${formatBytes(safeMetrics.memory.total)}</span>
                    <span>${formatBytes(safeMetrics.memory.free)} 可用</span>
                </div>
            </div>

            <!-- 磁盘 -->
            <div class="metric-card">
                <div class="metric-header">
                    <span class="metric-title">磁盘使用率</span>
                    <div class="metric-icon disk">💿</div>
                </div>
                <div class="metric-value">${safeMetrics.disk.usage_percent.toFixed(1)}%</div>
                <div class="metric-bar">
                    <div class="metric-bar-fill bar-disk" style="width: ${safeMetrics.disk.usage_percent}%"></div>
                </div>
                <div class="metric-detail">
                    <span>${formatBytes(safeMetrics.disk.used)} / ${formatBytes(safeMetrics.disk.total)}</span>
                    <span>${formatBytes(safeMetrics.disk.free)} 可用</span>
                </div>
                <div class="metric-detail" style="margin-top: 8px;">
                    <span>读: ${formatSpeed(safeMetrics.disk.read_speed)}</span>
                    <span>写: ${formatSpeed(safeMetrics.disk.write_speed)}</span>
                </div>
            </div>

            <!-- 网络 -->
            <div class="metric-card">
                <div class="metric-header">
                    <span class="metric-title">网络监控</span>
                    <div class="metric-icon network">🌐</div>
                </div>
                <div class="metric-detail" style="margin-bottom: 10px;">
                    <span style="font-weight: 600; color: #5e72e4;">⬆️ 上行速度: ${formatSpeed(safeMetrics.network.upload_speed)}</span>
                </div>
                <div class="metric-detail" style="margin-bottom: 12px;">
                    <span style="font-weight: 600; color: #2dce89;">⬇️ 下行速度: ${formatSpeed(safeMetrics.network.download_speed)}</span>
                </div>
                <div class="metric-detail" style="border-top: 1px solid #f0f0f0; padding-top: 10px;">
                    <span style="color: #5e72e4; font-weight: 600;">↑ 上行流量: ${formatBytes(safeMetrics.network.bytes_sent)}</span>
                    <span style="color: #2dce89; font-weight: 600;">↓ 下行流量: ${formatBytes(safeMetrics.network.bytes_recv)}</span>
                </div>
            </div>
        </div>

        <div style="text-align: center; color: #8898aa; margin-top: 20px; font-size: 14px;">
            💡 提示: 数据实时从远程主机获取，不在本地存储
        </div>
    `;
}

// 加载远程主机的 Docker 数据
async function loadRemoteDocker() {
    try {
        const response = await fetch(`/api/remote/${currentHostId}/docker`);
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const dockerData = await response.json();
        renderDocker(dockerData);
        
    } catch (error) {
        console.error('加载远程 Docker 数据失败:', error);
        showError('无法加载 Docker 数据: ' + error.message);
    }
}

// 渲染 Docker 数据
function renderDocker(dockerData) {
    const content = document.getElementById('monitor-content');
    
    const containers = dockerData.containers || [];
    const images = dockerData.images || [];
    
    content.innerHTML = `
        <div style="margin-bottom: 20px;">
            <h3 style="color: #2d3748; margin-bottom: 15px;">🐳 Docker 容器 (${containers.length})</h3>
            <div style="overflow-x: auto;">
                <table style="width: 100%; border-collapse: collapse; table-layout: fixed;">
                    <thead>
                        <tr style="background: #f8f9fa;">
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 20%;">容器名称</th>
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 35%;">镜像</th>
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 15%;">状态</th>
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 30%;">端口</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${containers.length === 0 ? 
                            '<tr><td colspan="4" style="padding: 20px; text-align: center; color: #8898aa;">暂无容器</td></tr>' :
                            containers.map(c => {
                                const ports = (c.ports && c.ports.length > 0) ? c.ports.join(', ') : '-';
                                const state = c.state || 'unknown';
                                return `
                                    <tr style="border-bottom: 1px solid #f0f0f0;">
                                        <td style="padding: 12px; word-break: break-all; white-space: normal;">${c.name || '-'}</td>
                                        <td style="padding: 12px; word-break: break-all; white-space: normal;">${c.image || '-'}</td>
                                        <td style="padding: 12px;">
                                            <span style="color: ${state === 'running' ? '#2dce89' : '#f5365c'};">
                                                ${state}
                                            </span>
                                        </td>
                                        <td style="padding: 12px; word-break: break-all; white-space: normal;">${ports}</td>
                                    </tr>
                                `;
                            }).join('')
                        }
                    </tbody>
                </table>
            </div>
        </div>

        <div>
            <h3 style="color: #2d3748; margin-bottom: 15px;">📦 Docker 镜像 (${images.length})</h3>
            <div style="overflow-x: auto;">
                <table style="width: 100%; border-collapse: collapse; table-layout: fixed;">
                    <thead>
                        <tr style="background: #f8f9fa;">
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 35%;">仓库</th>
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 20%;">标签</th>
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 20%;">大小</th>
                            <th style="padding: 12px; text-align: left; border-bottom: 2px solid #e9ecef; width: 25%;">创建时间</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${images.length === 0 ?
                            '<tr><td colspan="4" style="padding: 20px; text-align: center; color: #8898aa;">暂无镜像</td></tr>' :
                            images.map(img => `
                                <tr style="border-bottom: 1px solid #f0f0f0;">
                                    <td style="padding: 12px; word-break: break-all; white-space: normal;">${img.repository || '-'}</td>
                                    <td style="padding: 12px; word-break: break-all; white-space: normal;">${img.tag || '-'}</td>
                                    <td style="padding: 12px;">${img.size || '-'}</td>
                                    <td style="padding: 12px;">${img.created || '-'}</td>
                                </tr>
                            `).join('')
                        }
                    </tbody>
                </table>
            </div>
        </div>
    `;
}

// ==================== 主机操作 ====================

// 打开主机监控页面
async function openHostLink(hostId, event) {
    event.stopPropagation();
    
    try {
        // 获取主机信息
        const response = await fetch('/api/remote/hosts');
        const result = await response.json();
        
        if (!result.success) {
            alert('获取主机信息失败');
            return;
        }
        
        const host = result.data.find(h => h.id === hostId);
        if (!host) {
            alert('主机不存在');
            return;
        }
        
        // 构造远程主机的基础 URL
        const remoteBaseUrl = `http://${host.address}:${host.port}`;
        
        // 构造包含认证信息的特殊 URL 参数
        const authParams = new URLSearchParams({
            auto_username: host.username,
            auto_password: host.password,
            auto_login: 'true',
            timestamp: Date.now()
        });
        
        const remoteUrl = `${remoteBaseUrl}?${authParams.toString()}`;
        
        // 在新窗口打开远程主机
        window.open(remoteUrl, '_blank');
        
    } catch (error) {
        console.error('打开主机链接异常:', error);
        alert('操作失败: ' + error.message);
    }
}

// 显示添加主机模态框
function showAddHostModal() {
    document.getElementById('modal-title').textContent = '添加远程主机';
    document.getElementById('host-modal').classList.add('show');
    
    // 清空表单
    document.getElementById('host-name').value = '';
    document.getElementById('host-address').value = '';
    document.getElementById('host-port').value = '80';
    document.getElementById('host-username').value = '';
    document.getElementById('host-password').value = '';
}

// 编辑主机
async function editHost(hostId, event) {
    event.stopPropagation();
    
    try {
        const response = await fetch('/api/remote/hosts');
        const result = await response.json();
        
        if (!result.success) {
            alert('获取主机信息失败');
            return;
        }
        
        const host = result.data.find(h => h.id === hostId);
        if (!host) {
            alert('主机不存在');
            return;
        }
        
        // 填充表单
        document.getElementById('modal-title').textContent = '编辑远程主机';
        document.getElementById('host-modal').classList.add('show');
        document.getElementById('host-name').value = host.name;
        document.getElementById('host-address').value = host.address;
        document.getElementById('host-port').value = host.port;
        document.getElementById('host-username').value = host.username;
        document.getElementById('host-password').value = ''; // 密码不回显
        
        // 保存编辑的主机 ID
        document.getElementById('host-modal').dataset.editId = hostId;
        
    } catch (error) {
        console.error('编辑主机异常:', error);
        alert('操作失败');
    }
}

// 删除主机
async function deleteHost(hostId, event) {
    event.stopPropagation();
    
    if (!confirm('确定要删除这台主机吗？删除后无法恢复！')) {
        return;
    }
    
    try {
        const response = await fetch(`/api/remote/hosts/${hostId}`, {
            method: 'DELETE'
        });
        
        const result = await response.json();
        
        if (result.success) {
            alert('删除成功');
            
            // 如果删除的是当前选中的主机，清空选中状态
            if (currentHostId === hostId) {
                currentHostId = null;
                showEmptyState('请选择一个远程主机');
            }
            
            // 重新加载主机列表
            loadHostList();
        } else {
            alert('删除失败: ' + result.message);
        }
        
    } catch (error) {
        console.error('删除主机异常:', error);
        alert('操作失败');
    }
}

// 保存主机
async function saveHost() {
    const name = document.getElementById('host-name').value.trim();
    const address = document.getElementById('host-address').value.trim();
    const port = parseInt(document.getElementById('host-port').value);
    const username = document.getElementById('host-username').value.trim();
    const password = document.getElementById('host-password').value;
    
    if (!name || !address || !username || !password) {
        alert('请填写所有必填项');
        return;
    }
    
    const editId = document.getElementById('host-modal').dataset.editId;
    const isEdit = !!editId;
    
    const hostData = { name, address, port, username, password };
    
    try {
        const url = isEdit ? `/api/remote/hosts/${editId}` : '/api/remote/hosts';
        const method = isEdit ? 'PUT' : 'POST';
        
        const response = await fetch(url, {
            method: method,
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(hostData)
        });
        
        const result = await response.json();
        
        if (result.success) {
            alert(isEdit ? '更新成功' : '添加成功');
            closeModal();
            
            // 重新加载主机列表
            loadHostList();
        } else {
            alert('操作失败: ' + result.message);
        }
        
    } catch (error) {
        console.error('保存主机异常:', error);
        alert('操作失败');
    }
}

// 关闭模态框
function closeModal() {
    document.getElementById('host-modal').classList.remove('show');
    document.getElementById('host-modal').removeAttribute('data-edit-id');
}

// ==================== 辅助函数 ====================

// 格式化字节
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i];
}

// 格式化速度（智能单位转换）
function formatSpeed(bytesPerSecond) {
    if (bytesPerSecond === 0) return '0 KB/s';
    
    const k = 1024;
    if (bytesPerSecond < k) {
        return bytesPerSecond.toFixed(2) + ' B/s';
    } else if (bytesPerSecond < k * k) {
        return (bytesPerSecond / k).toFixed(2) + ' KB/s';
    } else if (bytesPerSecond < k * k * k) {
        return (bytesPerSecond / (k * k)).toFixed(2) + ' MB/s';
    } else if (bytesPerSecond < k * k * k * k) {
        return (bytesPerSecond / (k * k * k)).toFixed(2) + ' GB/s';
    } else {
        return (bytesPerSecond / (k * k * k * k)).toFixed(2) + ' TB/s';
    }
}

// 显示空状态
function showEmptyState(icon, title, subtitle) {
    const content = document.getElementById('monitor-content');
    content.innerHTML = `
        <div class="empty-state">
            <div class="empty-icon">${icon || '📡'}</div>
            <div class="empty-text">${title || '请选择一个远程主机'}</div>
            ${subtitle ? `<div class="empty-subtext">${subtitle}</div>` : ''}
        </div>
    `;
}

// 显示错误
function showError(message) {
    const content = document.getElementById('monitor-content');
    content.innerHTML = `
        <div class="empty-state">
            <div class="empty-icon">❌</div>
            <div class="empty-text">加载失败</div>
            <div class="empty-subtext">${message}</div>
        </div>
    `;
}

// 更新统计数据
async function updateStats() {
    try {
        const response = await fetch('/api/remote/hosts');
        const result = await response.json();
        
        if (result.success) {
            const hosts = result.data || [];
            const statusResponse = await fetch('/api/remote/hosts/status/all');
            const statusResult = await statusResponse.json();
            
            let onlineCount = 0;
            let offlineCount = 0;
            
            if (statusResult.success) {
                const statuses = statusResult.data || {};
                Object.values(statuses).forEach(status => {
                    if (status.online) {
                        onlineCount++;
                    } else {
                        offlineCount++;
                    }
                });
            }
            
            document.getElementById('total-hosts').textContent = hosts.length;
            document.getElementById('online-hosts').textContent = onlineCount;
            document.getElementById('offline-hosts').textContent = offlineCount;
        }
        
    } catch (error) {
        console.error('更新统计数据失败:', error);
    }
}

// 点击模态框外部关闭
document.addEventListener('DOMContentLoaded', () => {
    const modal = document.getElementById('host-modal');
    if (modal) {
        modal.addEventListener('click', function(e) {
            if (e.target === this) {
                closeModal();
            }
        });
    }
});
