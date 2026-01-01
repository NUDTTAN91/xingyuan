/*
 * 星垣 - CPU监控采集
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// collectCPU 采集CPU指标
func (c *Collector) collectCPU() (*CPUMetrics, error) {
	// 使用较短的间隔来减少阻塞时间
	percent, err := cpu.Percent(100*time.Millisecond, false)
	if err != nil {
		return nil, err
	}

	cores, err := cpu.Counts(true)
	if err != nil {
		cores = 0
	}

	times, err := cpu.Times(false)
	if err == nil && len(times) > 0 {
		total := times[0].User + times[0].System + times[0].Idle + times[0].Nice + times[0].Iowait
		return &CPUMetrics{
			UsagePercent:  roundFloat(percent[0], 2),
			Cores:         cores,
			UserPercent:   roundFloat((times[0].User/total)*100, 2),
			SystemPercent: roundFloat((times[0].System/total)*100, 2),
			IdlePercent:   roundFloat((times[0].Idle/total)*100, 2),
		}, nil
	}

	return &CPUMetrics{
		UsagePercent: roundFloat(percent[0], 2),
		Cores:        cores,
	}, nil
}
