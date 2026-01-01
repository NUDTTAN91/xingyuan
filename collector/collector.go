/*
 * 星垣 - 数据采集模块（主采集器）
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

// NetworkIOStat 网络 IO 统计
type NetworkIOStat struct {
	BytesRecv uint64
	BytesSent uint64
}

// Collector 数据采集器
type Collector struct {
	lastNetIO          *NetworkIOStat
	lastDiskIO         *disk.IOCountersStat
	lastNetTime        time.Time
	lastDiskTime       time.Time
	networkBaseline    *NetworkBaseline // 网络流量基准值
}

// NetworkBaseline 网络流量基准值
type NetworkBaseline struct {
	BytesRecvBaseline uint64 // 下行基准值
	BytesSentBaseline uint64 // 上行基准值
	LastRecv          uint64 // 上次读取的下行值
	LastSent          uint64 // 上次读取的上行值
}

// NewCollector 创建采集器实例
func NewCollector() *Collector {
	return &Collector{
		lastNetTime:  time.Now(),
		lastDiskTime: time.Now(),
		networkBaseline: &NetworkBaseline{
			BytesRecvBaseline: 0,
			BytesSentBaseline: 0,
			LastRecv:          0,
			LastSent:          0,
		},
	}
}

// LoadBaseline 加载网络流量基准值（从数据库）
func (c *Collector) LoadBaseline(recvBaseline, sentBaseline, lastRecv, lastSent uint64) {
	c.networkBaseline = &NetworkBaseline{
		BytesRecvBaseline: recvBaseline,
		BytesSentBaseline: sentBaseline,
		LastRecv:          lastRecv,
		LastSent:          lastSent,
	}
}

// GetBaseline 获取当前的网络基准值
func (c *Collector) GetBaseline() *NetworkBaseline {
	return c.networkBaseline
}

// Collect 采集所有监控数据
func (c *Collector) Collect() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		Timestamp: time.Now().Unix(),
	}

	// 采集CPU数据
	cpuMetrics, err := c.collectCPU()
	if err == nil {
		metrics.CPU = *cpuMetrics
	}

	// 采集内存数据
	memMetrics, err := c.collectMemory()
	if err == nil {
		metrics.Memory = *memMetrics
	}

	// 采集磁盘数据
	diskMetrics, err := c.collectDisk()
	if err == nil {
		metrics.Disk = *diskMetrics
	}

	// 采集网络数据
	netMetrics, err := c.collectNetwork()
	if err == nil {
		metrics.Network = *netMetrics
	}

	// 采集进程数据
	processes, err := c.collectProcesses(10)
	if err == nil {
		metrics.Processes = processes
	}

	// 采集系统信息
	sysInfo, err := c.collectSystemInfo()
	if err == nil {
		metrics.SystemInfo = *sysInfo
	}

	return metrics, nil
}

// CollectDocker 采集Docker监控数据
func (c *Collector) CollectDocker() (*DockerMetrics, error) {
	return c.collectDocker()
}
