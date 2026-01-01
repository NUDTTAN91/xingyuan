/*
 * 星垣 - Docker监控页面脚本
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

let refreshTimer = null;

// 更新容器表格
function updateContainerTable(containers) {
    const tbody = document.getElementById('container-table');
    tbody.innerHTML = '';

    if (!containers || containers.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; color: #95a5a6;">暂无容器</td></tr>';
        return;
    }

    containers.forEach(container => {
        const stateClass = {
            'running': 'status-running',
            'exited': 'status-exited',
            'paused': 'status-paused',
            'restarting': 'status-restarting',
            'created': 'status-created'
        }[container.state] || 'status-exited';

        const stateText = {
            'running': '🟢 运行中',
            'exited': '⚫ 已停止',
            'paused': '⏸️ 已暂停',
            'restarting': '🔄 重启中',
            'created': '🆕 已创建'
        }[container.state] || '未知';

        const portsHtml = container.ports && container.ports.length > 0 
            ? container.ports.map(p => `<div>${p}</div>`).join('')
            : '-';

        const row = document.createElement('tr');
        row.innerHTML = `
            <td><span class="container-id">${container.id}</span></td>
            <td><strong>${container.image}</strong></td>
            <td>${container.name}</td>
            <td><span class="status-badge ${stateClass}">${stateText}</span></td>
            <td><span class="port-info">${portsHtml}</span></td>
            <td>${container.created}</td>
            <td>
                <div class="action-buttons">
                    ${container.state === 'running' 
                        ? `<button class="action-btn stop-btn" onclick="stopContainer('${container.id}', '${container.name}')" title="停止容器">⏸️ 停止</button>` 
                        : `<button class="action-btn restart-btn" onclick="restartContainer('${container.id}', '${container.name}')" title="重启容器">▶️ 重启</button>`
                    }
                    <button class="action-btn delete-btn" onclick="deleteContainer('${container.id}', '${container.name}')" title="删除容器">🗑️ 删除</button>
                </div>
            </td>
        `;
        tbody.appendChild(row);
    });

    document.getElementById('container-list-count').textContent = containers.length;
}

// 更新镜像表格
function updateImageTable(images) {
    const tbody = document.getElementById('image-table');
    tbody.innerHTML = '';

    if (!images || images.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align: center; color: #95a5a6;">暂无镜像</td></tr>';
        return;
    }

    images.forEach(image => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td><strong>${image.repository}</strong></td>
            <td><span class="badge badge-success">${image.tag}</span></td>
            <td><span class="image-id">${image.id}</span></td>
            <td>${image.size}</td>
            <td>${image.created}</td>
        `;
        tbody.appendChild(row);
    });

    document.getElementById('image-list-count').textContent = images.length;
}

// 手动刷新
function manualRefresh() {
    console.log('手动刷新数据...');
    loadDockerData();
}

// 视图切换
function switchView(view) {
    // 切换按钮状态
    const buttons = document.querySelectorAll('.switch-btn');
    buttons.forEach(btn => btn.classList.remove('active'));
    event.target.classList.add('active');

    // 切换表格显示
    document.getElementById('container-view').classList.remove('active');
    document.getElementById('image-view').classList.remove('active');
    
    if (view === 'container') {
        document.getElementById('container-view').classList.add('active');
    } else {
        document.getElementById('image-view').classList.add('active');
    }
}

// 设置刷新间隔(从预设按钮)
function setRefreshInterval(value, unit) {
    document.getElementById('refresh-interval-input').value = value;
    document.getElementById('refresh-unit').value = unit;
    updateRefreshIntervalFromInput();
}

// 从输入框更新刷新间隔
function updateRefreshIntervalFromInput() {
    const value = parseInt(document.getElementById('refresh-interval-input').value) || 0;
    const unit = parseInt(document.getElementById('refresh-unit').value) || 1;
    const intervalSeconds = value * unit; // 转换为秒
    
    // 清除旧的定时器
    if (refreshTimer) {
        clearInterval(refreshTimer);
        refreshTimer = null;
    }

    // 更新状态显示
    const statusEl = document.getElementById('auto-refresh-status');
    if (intervalSeconds === 0) {
        statusEl.textContent = '● 自动刷新: 已关闭';
        statusEl.style.background = '#e8e8e8';
        statusEl.style.color = '#6c757d';
    } else {
        let timeText = '';
        if (unit === 3600) {
            timeText = `${value}小时`;
        } else if (unit === 60) {
            timeText = `${value}分钟`;
        } else {
            timeText = `${value}秒`;
        }
        statusEl.textContent = `● 自动刷新: ${timeText}`;
        statusEl.style.background = '#d4f4dd';
        statusEl.style.color = '#1f7a3e';

        // 设置新的定时器
        refreshTimer = setInterval(loadDockerData, intervalSeconds * 1000);
    }
}

// 加载Docker数据
function loadDockerData() {
    // 从 API 获取真实数据
    fetch('/api/docker')
        .then(response => response.json())
        .then(data => {
            // 更新统计数据
            document.getElementById('container-count').textContent = data.container_count || 0;
            document.getElementById('running-count').textContent = data.running_count || 0;
            document.getElementById('image-count').textContent = data.image_count || 0;
            
            // 更新表格
            updateContainerTable(data.containers || []);
            updateImageTable(data.images || []);
            
            // 更新时间
            const now = new Date();
            document.getElementById('update-time').textContent = now.toLocaleString('zh-CN');
            
            // 更新连接状态
            document.getElementById('connection-status').textContent = '● 已连接';
            document.getElementById('connection-status').className = 'connection-status status-connected';
        })
        .catch(error => {
            console.error('Error fetching docker data:', error);
            document.getElementById('connection-status').textContent = '● 已断开';
            document.getElementById('connection-status').className = 'connection-status status-disconnected';
        });
}

// 页面加载时初始化
window.addEventListener('load', function() {
    loadDockerData();
    updateRefreshIntervalFromInput(); // 启动自动刷新
});

// 页面关闭时清理定时器
window.addEventListener('beforeunload', function() {
    if (refreshTimer) {
        clearInterval(refreshTimer);
    }
});

// 停止容器
function stopContainer(containerId, containerName) {
    if (!confirm(`确定要停止容器 "${containerName}" 吗？`)) {
        return;
    }
    
    // 显示加载提示
    const statusEl = document.getElementById('connection-status');
    const originalText = statusEl.textContent;
    statusEl.textContent = '● 正在停止容器...';
    statusEl.className = 'connection-status status-disconnected';
    
    fetch('/api/docker/container/stop', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ container_id: containerId })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            alert(`容器 "${containerName}" 已成功停止`);
            // 立即刷新数据
            loadDockerData();
        } else {
            alert(`停止容器失败: ${data.error || '未知错误'}`);
        }
    })
    .catch(error => {
        console.error('Error stopping container:', error);
        alert(`停止容器失败: ${error.message}`);
    })
    .finally(() => {
        statusEl.textContent = originalText;
        statusEl.className = 'connection-status status-connected';
    });
}

// 删除容器
function deleteContainer(containerId, containerName) {
    if (!confirm(`警告：删除容器 "${containerName}" 是不可逆操作！\n\n确定要继续吗？`)) {
        return;
    }
    
    // 显示加载提示
    const statusEl = document.getElementById('connection-status');
    const originalText = statusEl.textContent;
    statusEl.textContent = '● 正在删除容器...';
    statusEl.className = 'connection-status status-disconnected';
    
    fetch('/api/docker/container/delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ container_id: containerId })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            alert(`容器 "${containerName}" 已成功删除`);
            // 立即刷新数据
            loadDockerData();
        } else {
            alert(`删除容器失败: ${data.error || '未知错误'}`);
        }
    })
    .catch(error => {
        console.error('Error deleting container:', error);
        alert(`删除容器失败: ${error.message}`);
    })
    .finally(() => {
        statusEl.textContent = originalText;
        statusEl.className = 'connection-status status-connected';
    });
}

// 重启容器
function restartContainer(containerId, containerName) {
    if (!confirm(`确定要重启容器 "${containerName}" 吗？`)) {
        return;
    }
    
    // 显示加载提示
    const statusEl = document.getElementById('connection-status');
    const originalText = statusEl.textContent;
    statusEl.textContent = '● 正在重启容器...';
    statusEl.className = 'connection-status status-disconnected';
    
    fetch('/api/docker/container/restart', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ container_id: containerId })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            alert(`容器 "${containerName}" 已成功重启`);
            // 立即刷新数据
            loadDockerData();
        } else {
            alert(`重启容器失败: ${data.error || '未知错误'}`);
        }
    })
    .catch(error => {
        console.error('Error restarting container:', error);
        alert(`重启容器失败: ${error.message}`);
    })
    .finally(() => {
        statusEl.textContent = originalText;
        statusEl.className = 'connection-status status-connected';
    });
}
