/*
 * 星垣 - 工具函数
 * Author: tan91
 * GitHub: https://github.com/NUDTTAN91
 * Blog: https://blog.csdn.net/ZXW_NUDT
 * Project: https://github.com/NUDTTAN91/xingyuan
 */

package collector

import "math"

// roundFloat 浮点数四舍五入
func roundFloat(val float64, precision int) float64 {
	p := math.Pow10(precision)
	return math.Round(val*p) / p
}

// roundFloat32 32位浮点数四舍五入
func roundFloat32(val float32, precision int) float32 {
	p := float32(math.Pow10(precision))
	return float32(math.Round(float64(val*p))) / p
}

// getProcessStatus 获取进程状态中文描述
func getProcessStatus(status []string) string {
	if len(status) == 0 {
		return "运行中"
	}

	statusMap := map[string]string{
		"R": "运行中",
		"S": "睡眠",
		"T": "停止",
		"Z": "僵尸",
		"D": "不可中断",
	}
	if s, ok := statusMap[status[0]]; ok {
		return s
	}
	return "运行中"
}
