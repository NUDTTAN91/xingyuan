/*
 * 星垣 - 进程监控采集
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import (
	"sort"

	"github.com/shirou/gopsutil/v3/process"
)

// collectProcesses 采集进程信息
func (c *Collector) collectProcesses(topN int) ([]ProcessInfo, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	var procList []ProcessInfo
	for _, p := range processes {
		name, _ := p.Name()
		cpuPercent, _ := p.CPUPercent()
		memPercent, _ := p.MemoryPercent()
		memInfo, _ := p.MemoryInfo()
		status, _ := p.Status()

		// 过滤空进程名
		if name == "" {
			continue
		}

		procInfo := ProcessInfo{
			PID:        p.Pid,
			Name:       name,
			CPUPercent: roundFloat(cpuPercent, 2),
			MemPercent: roundFloat32(memPercent, 2),
			Status:     getProcessStatus(status),
		}
		if memInfo != nil {
			procInfo.MemoryUsage = memInfo.RSS
		}
		procList = append(procList, procInfo)
	}

	// 按CPU使用率降序排序，取前N个
	sort.Slice(procList, func(i, j int) bool {
		return procList[i].CPUPercent > procList[j].CPUPercent
	})
	if len(procList) > topN {
		procList = procList[:topN]
	}

	return procList, nil
}
