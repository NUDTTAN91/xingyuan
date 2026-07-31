package database

import (
	"database/sql"
	"log"
	"os"
)

// CheckTimezoneChange 时区一致性检查
// 时区由 docker-compose.yml 的 TZ 环境变量决定，监控数据时间戳按该时区记录。
// 首次启动时记录当前时区；后续启动若发现 TZ 变更，打印醒目警告并更新记录。
func CheckTimezoneChange() {
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "(未设置)"
	}

	var stored string
	err := db.QueryRow("SELECT value FROM system_meta WHERE key = 'timezone'").Scan(&stored)
	if err == sql.ErrNoRows {
		// 首次启动：记录当前时区
		_, err := db.Exec("INSERT INTO system_meta (key, value, updated_at) VALUES ('timezone', ?, datetime('now'))", tz)
		if err != nil {
			log.Printf("记录时区信息失败: %v", err)
			return
		}
		log.Printf("当前时区: %s（由 docker-compose.yml 的 TZ 环境变量决定）", tz)
		return
	}
	if err != nil {
		log.Printf("读取时区信息失败: %v", err)
		return
	}

	if stored != tz {
		log.Printf("【警告】时区已从 %q 变更为 %q！历史监控数据的时间戳仍按旧时区记录，"+
			"图表上新老数据的时间轴将出现偏移。如非有意变更，请恢复 docker-compose.yml 中的 TZ 配置", stored, tz)
		if _, err := db.Exec("UPDATE system_meta SET value = ?, updated_at = datetime('now') WHERE key = 'timezone'", tz); err != nil {
			log.Printf("更新时区信息失败: %v", err)
		}
	}
}
