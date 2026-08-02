/*
 * 星垣 - 监控脚本
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

let ws = null;
let reconnectTimer = null;

/**
 * 连接WebSocket
 */
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const token = getAccessToken(); // 从 auth.js 获取 Token
    
    // Token 通过 Sec-WebSocket-Protocol 子协议传递（避免出现在URL和访问日志中）
    const wsUrl = `${protocol}//${window.location.host}/api/ws`;
    
    ws = token
        ? new WebSocket(wsUrl, ['xingyuan-auth', token])
        : new WebSocket(wsUrl);

    ws.onopen = function() {
        console.log('WebSocket connected');
        updateConnectionStatus(true);
        if (reconnectTimer) {
            clearTimeout(reconnectTimer);
            reconnectTimer = null;
        }
    };

    ws.onmessage = function(event) {
        const data = JSON.parse(event.data);
        updateMetrics(data);
    };

    ws.onerror = function(error) {
        console.error('WebSocket error:', error);
        updateConnectionStatus(false);
    };

    ws.onclose = function() {
        console.log('WebSocket disconnected');
        updateConnectionStatus(false);
        // 重连前先校验认证状态：Token失效时由fetch拦截器自动刷新，
        // 刷新失败则跳转登录页（避免后台标签页拿旧Token无限重连）
        fetch('/api/verify').catch(() => {}).finally(() => {
            // 5秒后重连
            reconnectTimer = setTimeout(connectWebSocket, 5000);
        });
    };
}

/**
 * 更新连接状态
 */
function updateConnectionStatus(connected) {
    const statusEl = document.getElementById('connection-status');
    if (connected) {
        statusEl.textContent = '● 已连接';
        statusEl.className = 'connection-status status-connected';
    } else {
        statusEl.textContent = '● 已断开';
        statusEl.className = 'connection-status status-disconnected';
    }
}

/**
 * 更新所有监控指标
 */
function updateMetrics(data) {
    // 更新系统信息
    document.getElementById('hostname').textContent = data.system_info.hostname || '-';
    document.getElementById('platform').textContent = data.system_info.platform || '-';
    document.getElementById('uptime').textContent = formatUptime(data.system_info.uptime);
    
    // 更新数据库统计信息（实时更新）
    if (data.database_info) {
        document.getElementById('db-records').textContent = data.database_info.total_records.toLocaleString('zh-CN') + ' 条';
        document.getElementById('db-size').textContent = formatBytes(data.database_info.data_size);
    }

    // 更新CPU
    document.getElementById('cpu-value').textContent = data.cpu.usage_percent.toFixed(1) + '%';
    document.getElementById('cpu-bar').style.width = data.cpu.usage_percent + '%';
    document.getElementById('cpu-cores').textContent = data.cpu.cores + ' 核心';
    document.getElementById('cpu-idle').textContent = data.cpu.idle_percent.toFixed(1) + '% 空闲';

    // 更新内存
    document.getElementById('mem-value').textContent = data.memory.usage_percent.toFixed(1) + '%';
    document.getElementById('mem-bar').style.width = data.memory.usage_percent + '%';
    document.getElementById('mem-used').textContent = formatBytes(data.memory.used) + ' / ' + formatBytes(data.memory.total);
    document.getElementById('mem-free').textContent = formatBytes(data.memory.available) + ' 可用';

    // 更新磁盘
    document.getElementById('disk-value').textContent = data.disk.usage_percent.toFixed(1) + '%';
    document.getElementById('disk-bar').style.width = data.disk.usage_percent + '%';
    document.getElementById('disk-used').textContent = formatBytes(data.disk.used) + ' / ' + formatBytes(data.disk.total);
    document.getElementById('disk-free').textContent = formatBytes(data.disk.free) + ' 可用';
    const diskRead = formatSpeed(data.disk.read_speed);
    const diskWrite = formatSpeed(data.disk.write_speed);
    document.getElementById('disk-read').textContent = '读: ' + diskRead.value + ' ' + diskRead.unit;
    document.getElementById('disk-write').textContent = '写: ' + diskWrite.value + ' ' + diskWrite.unit;

    // 更新网络
    const downSpeed = formatSpeed(data.network.download_speed);
    const upSpeed = formatSpeed(data.network.upload_speed);
    document.getElementById('net-up').innerHTML = upSpeed.value + ' <span class="network-unit">' + upSpeed.unit + '</span>';
    document.getElementById('net-down').innerHTML = downSpeed.value + ' <span class="network-unit">' + downSpeed.unit + '</span>';
    
    // 更新累计流量（按照UI规范：先上行后下行）
    document.getElementById('net-sent-total').textContent = '↑ 上行流量: ' + formatBytes(data.network.bytes_sent);
    document.getElementById('net-recv-total').textContent = '↓ 下行流量: ' + formatBytes(data.network.bytes_recv);

    // 更新进程表
    updateProcessTable(data.processes);

    // 更新时间
    const now = new Date();
    document.getElementById('update-time').textContent = now.toLocaleString('zh-CN');
}

/**
 * 更新进程表格
 */
function updateProcessTable(processes) {
    const tbody = document.getElementById('process-table');
    tbody.innerHTML = '';

    if (!processes || processes.length === 0) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align: center; color: #95a5a6;">暂无数据</td></tr>';
        return;
    }

    processes.forEach(proc => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td><strong>${proc.name}</strong></td>
            <td>${proc.pid}</td>
            <td>${proc.cpu_percent.toFixed(1)}% <div class="progress-mini"><div class="progress-mini-fill" style="width: ${Math.min(proc.cpu_percent, 100)}%"></div></div></td>
            <td>${proc.mem_percent.toFixed(1)}%</td>
            <td>${formatBytes(proc.memory_usage)}</td>
            <td><span class="badge badge-success">${proc.status}</span></td>
        `;
        tbody.appendChild(row);
    });
}

/**
 * 格式化字节数
 */
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i];
}

/**
 * 格式化速度（字节/秒）
 */
function formatSpeed(bytesPerSec) {
    if (bytesPerSec === 0) return { value: '0', unit: 'B/s' };
    const k = 1024;
    const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
    const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
    return {
        value: (bytesPerSec / Math.pow(k, i)).toFixed(1),
        unit: sizes[i]
    };
}

/**
 * 格式化运行时间
 */
function formatUptime(seconds) {
    if (!seconds) return '-';
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    return `${days}天 ${hours}小时 ${minutes}分`;
}

// 页面加载时连接WebSocket
window.addEventListener('load', function() {
    connectWebSocket();
});

// 页面关闭时关闭WebSocket
window.addEventListener('beforeunload', function() {
    if (ws) {
        ws.close();
    }
});
