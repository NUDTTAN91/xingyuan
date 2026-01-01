/*
 * 星垣 - 认证工具库
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

// ==================== Token 管理 ====================

// 获取 Access Token
function getAccessToken() {
    return localStorage.getItem('access_token');
}

// 获取 Refresh Token
function getRefreshToken() {
    return localStorage.getItem('refresh_token');
}

// 设置 Token
function setTokens(accessToken, refreshToken) {
    localStorage.setItem('access_token', accessToken);
    if (refreshToken) {
        localStorage.setItem('refresh_token', refreshToken);
    }
}

// 清除 Token
function clearTokens() {
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
}

// ==================== 认证检查 ====================

// 检查是否已登录
async function checkAuth() {
    const token = getAccessToken();
    
    // 如果没有 Token，跳转登录页
    if (!token) {
        redirectToLogin();
        return false;
    }
    
    // 验证 Token 是否有效
    try {
        const response = await fetch('/api/verify', {
            headers: {
                'Authorization': 'Bearer ' + token
            }
        });
        
        if (response.ok) {
            return true;
        } else {
            // Token 无效，尝试刷新
            const refreshed = await refreshAccessToken();
            if (!refreshed) {
                redirectToLogin();
                return false;
            }
            return true;
        }
    } catch (error) {
        console.error('认证检查失败:', error);
        redirectToLogin();
        return false;
    }
}



// 跳转到登录页
function redirectToLogin() {
    // 保存当前页面 URL，登录后跳回
    const currentPath = window.location.pathname + window.location.search;
    if (currentPath !== '/static/login.html') {
        sessionStorage.setItem('redirect_after_login', currentPath);
    }
    window.location.href = '/static/login.html';
}

// ==================== API 请求封装 ====================

// 带认证的 fetch 请求（已废弃，使用全局 fetch 拦截器代替）
async function authenticatedFetch(url, options = {}) {
    return fetch(url, options);
}

// ==================== 登出 ====================

// 登出
async function logout() {
    const token = getAccessToken();
    const refreshToken = getRefreshToken();
    
    if (token) {
        try {
            // 使用原始 fetch 避免拦截
            await window.fetch('/api/logout', {
                method: 'POST',
                headers: {
                    'Authorization': 'Bearer ' + token,
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    refresh_token: refreshToken
                })
            });
        } catch (error) {
            console.error('登出请求失败:', error);
        }
    }
    
    // 清除本地 Token
    clearTokens();
    
    // 跳转登录页
    window.location.href = '/static/login.html';
}

// ==================== 全局 fetch 拦截器 ====================

// 保存原始 fetch
const originalFetch = window.fetch;

// 刷新 Access Token（使用原始 fetch）
async function refreshAccessToken() {
    const refreshToken = getRefreshToken();
    
    if (!refreshToken) {
        return false;
    }
    
    try {
        const response = await originalFetch('/api/refresh', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                refresh_token: refreshToken
            })
        });
        
        if (response.ok) {
            const data = await response.json();
            if (data.success) {
                setTokens(data.access_token, data.refresh_token);
                return true;
            }
        }
        
        return false;
    } catch (error) {
        console.error('刷新 Token 失败:', error);
        return false;
    }
}

// 重写全局 fetch，自动处理认证
window.fetch = async function(url, options = {}) {
    // 如果是 API 请求，自动添加 Authorization 头
    if (typeof url === 'string' && url.startsWith('/api/')) {
        const token = getAccessToken();
        if (token) {
            if (!options.headers) {
                options.headers = {};
            }
            options.headers['Authorization'] = 'Bearer ' + token;
        }
    }
    
    try {
        let response = await originalFetch(url, options);
        
        // 如果返回 401，尝试刷新 Token
        if (response.status === 401 && typeof url === 'string' && url.startsWith('/api/')) {
            // 排除登录、刷新等公开 API
            if (url === '/api/login' || url === '/api/refresh') {
                return response;
            }
            
            const refreshed = await refreshAccessToken();
            if (refreshed) {
                // 重试请求
                const newToken = getAccessToken();
                if (!options.headers) {
                    options.headers = {};
                }
                options.headers['Authorization'] = 'Bearer ' + newToken;
                response = await originalFetch(url, options);
            } else {
                // 刷新失败，跳转登录
                redirectToLogin();
            }
        }
        
        return response;
    } catch (error) {
        throw error;
    }
};

// ==================== 页面加载时自动检查 ====================

// 立即执行认证检查（IIFE）
(async function() {
    // 如果是登录页面，不检查认证
    if (window.location.pathname === '/static/login.html') {
        return;
    }
    
    const token = getAccessToken();
    
    // 如果没有 Token，立即跳转登录页
    if (!token) {
        redirectToLogin();
        return;
    }
    
    // 验证 Token 是否有效
    try {
        const response = await originalFetch('/api/verify', {
            headers: {
                'Authorization': 'Bearer ' + token
            }
        });
        
        if (!response.ok) {
            // Token 无效，尝试刷新
            const refreshed = await refreshAccessToken();
            if (!refreshed) {
                redirectToLogin();
            }
        }
    } catch (error) {
        console.error('认证检查失败:', error);
        // 网络错误不跳转，让页面继续加载
    }
})();


