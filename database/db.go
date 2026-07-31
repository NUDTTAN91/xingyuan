package database

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var (
	db   *sql.DB
	once sync.Once
)

// Init 初始化数据库连接
func Init(dataDir string) error {
	var err error
	once.Do(func() {
		dbPath := filepath.Join(dataDir, "monitor.db")
		// 添加 SQLite 连接参数：启用 WAL 模式、设置 busy_timeout
		dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", dbPath)
		db, err = sql.Open("sqlite3", dsn)
		if err != nil {
			err = fmt.Errorf("打开数据库失败: %v", err)
			return
		}

		// 设置连接池
		db.SetMaxOpenConns(1) // SQLite建议单连接
		db.SetMaxIdleConns(1)

		// 创建表结构
		if err = createTables(); err != nil {
			err = fmt.Errorf("创建表失败: %v", err)
			return
		}

		// 初始化总记录数计数器（启动时统计一次，之后增量维护）
		if err = initTotalRecords(); err != nil {
			err = fmt.Errorf("初始化记录数统计失败: %v", err)
			return
		}

		// 时区一致性检查（时区由 docker-compose.yml 的 TZ 决定）
		CheckTimezoneChange()

		log.Printf("数据库初始化成功: %s (WAL模式)", dbPath)
	})
	return err
}

// GetDB 获取数据库实例
func GetDB() *sql.DB {
	return db
}

// Close 关闭数据库连接
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
