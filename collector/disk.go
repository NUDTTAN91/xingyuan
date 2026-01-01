/*
 * 星垣 - 磁盘监控采集
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

// collectDisk 采集磁盘指标
func (c *Collector) collectDisk() (*DiskMetrics, error) {
	usage, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}

	metrics := &DiskMetrics{
		UsagePercent: roundFloat(usage.UsedPercent, 2),
		Total:        usage.Total,
		Used:         usage.Used,
		Free:         usage.Free,
	}

	// 计算磁盘读写速度
	ioCounters, err := disk.IOCounters()
	if err == nil && len(ioCounters) > 0 {
		now := time.Now()
		var totalRead, totalWrite uint64
		for _, io := range ioCounters {
			totalRead += io.ReadBytes
			totalWrite += io.WriteBytes
		}

		if c.lastDiskIO != nil {
			duration := now.Sub(c.lastDiskTime).Seconds()
			if duration > 0 {
				metrics.ReadSpeed = roundFloat(float64(totalRead-c.lastDiskIO.ReadBytes)/duration, 2)
				metrics.WriteSpeed = roundFloat(float64(totalWrite-c.lastDiskIO.WriteBytes)/duration, 2)
			}
		}

		c.lastDiskIO = &disk.IOCountersStat{
			ReadBytes:  totalRead,
			WriteBytes: totalWrite,
		}
		c.lastDiskTime = now
	}

	return metrics, nil
}
