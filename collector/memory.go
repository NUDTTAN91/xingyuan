/*
 * 星垣 - 内存监控采集
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import "github.com/shirou/gopsutil/v3/mem"

// collectMemory 采集内存指标
func (c *Collector) collectMemory() (*MemoryMetrics, error) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	return &MemoryMetrics{
		UsagePercent: roundFloat(v.UsedPercent, 2),
		Total:        v.Total,
		Used:         v.Used,
		Free:         v.Free,
		Available:    v.Available,
	}, nil
}
