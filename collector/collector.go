/*
 * 星垣 - 数据采集模块（主采集器）
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"log"
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
// 子采集失败时记录日志并标记到 FailedParts，返回部分数据（失败项为零值）
func (c *Collector) Collect() (*SystemMetrics, error) {
	metrics := &SystemMetrics{
		Timestamp: time.Now().Unix(),
	}

	// 采集CPU数据
	if cpuMetrics, err := c.collectCPU(); err == nil {
		metrics.CPU = *cpuMetrics
	} else {
		metrics.FailedParts = append(metrics.FailedParts, "cpu")
		log.Printf("采集CPU数据失败: %v", err)
	}

	// 采集内存数据
	if memMetrics, err := c.collectMemory(); err == nil {
		metrics.Memory = *memMetrics
	} else {
		metrics.FailedParts = append(metrics.FailedParts, "memory")
		log.Printf("采集内存数据失败: %v", err)
	}

	// 采集磁盘数据
	if diskMetrics, err := c.collectDisk(); err == nil {
		metrics.Disk = *diskMetrics
	} else {
		metrics.FailedParts = append(metrics.FailedParts, "disk")
		log.Printf("采集磁盘数据失败: %v", err)
	}

	// 采集网络数据
	if netMetrics, err := c.collectNetwork(); err == nil {
		metrics.Network = *netMetrics
	} else {
		metrics.FailedParts = append(metrics.FailedParts, "network")
		log.Printf("采集网络数据失败: %v", err)
	}

	// 采集进程数据
	if processes, err := c.collectProcesses(10); err == nil {
		metrics.Processes = processes
	} else {
		log.Printf("采集进程数据失败: %v", err)
	}

	// 采集系统信息
	if sysInfo, err := c.collectSystemInfo(); err == nil {
		metrics.SystemInfo = *sysInfo
	} else {
		log.Printf("采集系统信息失败: %v", err)
	}

	return metrics, nil
}

// CollectDocker 采集Docker监控数据
func (c *Collector) CollectDocker() (*DockerMetrics, error) {
	return c.collectDocker()
}
