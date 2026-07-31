package database

import (
	"os"
	"testing"
)

// TestMain 初始化测试数据库（包级单例，全部测试共用）
func TestMain(m *testing.M) {
	os.Setenv("TZ", "Asia/Shanghai")

	dir, err := os.MkdirTemp("", "xingyuan-dbtest-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	if err := Init(dir); err != nil {
		panic("测试数据库初始化失败: " + err.Error())
	}

	code := m.Run()
	Close()
	os.Exit(code)
}

// TestTimezoneChangeDetection 时区变更检测
func TestTimezoneChangeDetection(t *testing.T) {
	// Init 时 TZ=Asia/Shanghai，应已记录
	var stored string
	if err := db.QueryRow("SELECT value FROM system_meta WHERE key = 'timezone'").Scan(&stored); err != nil {
		t.Fatalf("查询时区记录失败: %v", err)
	}
	if stored != "Asia/Shanghai" {
		t.Fatalf("初始时区记录错误: %q", stored)
	}

	// 变更 TZ 后检测，记录应更新
	t.Setenv("TZ", "UTC")
	CheckTimezoneChange()

	if err := db.QueryRow("SELECT value FROM system_meta WHERE key = 'timezone'").Scan(&stored); err != nil {
		t.Fatalf("查询时区记录失败: %v", err)
	}
	if stored != "UTC" {
		t.Errorf("时区变更后记录应更新为 UTC, 实际: %q", stored)
	}

	// 恢复记录，避免影响其他测试语义
	t.Setenv("TZ", "Asia/Shanghai")
	CheckTimezoneChange()
}

// TestAggregateAndDelete 保留任务：超期数据按分钟压缩后删除，新数据不受影响
func TestAggregateAndDelete(t *testing.T) {
	// 老数据：同一分钟3条(10/20/30)+下一分钟1条(40)；新数据1条(99)
	oldRows := []struct {
		usage float64
		ts    string
	}{
		{10, "2020-01-01 08:00:01"},
		{20, "2020-01-01 08:00:30"},
		{30, "2020-01-01 08:00:59"},
		{40, "2020-01-01 08:01:10"},
	}
	for _, r := range oldRows {
		if _, err := db.Exec("INSERT INTO cpu_metrics (usage, timestamp) VALUES (?, ?)", r.usage, r.ts); err != nil {
			t.Fatalf("插入测试数据失败: %v", err)
		}
	}
	if _, err := db.Exec("INSERT INTO cpu_metrics (usage, timestamp) VALUES (99, '2099-01-01 00:00:00')"); err != nil {
		t.Fatalf("插入新数据失败: %v", err)
	}

	// 执行压缩：cutoff 晚于老数据、早于新数据
	deleted, err := aggregateAndDelete(aggregateTasks[0], "2020-06-01 00:00:00")
	if err != nil {
		t.Fatalf("压缩失败: %v", err)
	}
	if deleted != 4 {
		t.Errorf("应删除4条超期原始数据, 实际 %d", deleted)
	}

	// 聚合表应有2条分钟级记录，第一分钟均值20
	var aggCount int
	db.QueryRow("SELECT COUNT(*) FROM cpu_metrics_agg").Scan(&aggCount)
	if aggCount != 2 {
		t.Errorf("聚合表应有2条记录, 实际 %d", aggCount)
	}
	var avgUsage float64
	db.QueryRow("SELECT usage FROM cpu_metrics_agg WHERE timestamp = '2020-01-01 08:00:00'").Scan(&avgUsage)
	if avgUsage != 20 {
		t.Errorf("第一分钟均值应为20, 实际 %v", avgUsage)
	}

	// 新数据应保留在原始表
	var rawCount int
	db.QueryRow("SELECT COUNT(*) FROM cpu_metrics WHERE usage = 99").Scan(&rawCount)
	if rawCount != 1 {
		t.Errorf("保留期内数据不应被删除")
	}
}

// TestSampledQueryAcrossTables 采样查询应能同时命中原始表与聚合表
func TestSampledQueryAcrossTables(t *testing.T) {
	// 依赖 TestAggregateAndDelete 产生的数据布局：
	// 聚合表 2020-01-01 两条 + 原始表 2099-01-01 一条
	metrics, err := QueryCPUMetricsSampled("2019-01-01 00:00:00", "2099-12-31 23:59:59", 60)
	if err != nil {
		t.Fatalf("采样查询失败: %v", err)
	}
	if len(metrics) < 3 {
		t.Errorf("联合查询应至少返回3个点(2老+1新), 实际 %d", len(metrics))
	}

	// 结果应按时间升序
	for i := 1; i < len(metrics); i++ {
		if metrics[i].Timestamp < metrics[i-1].Timestamp {
			t.Errorf("结果未按时间升序: %s 在 %s 之后", metrics[i].Timestamp, metrics[i-1].Timestamp)
		}
	}
}

// TestInsertUpdatesCounter 插入应增量维护总记录数计数器
func TestInsertUpdatesCounter(t *testing.T) {
	before, err := GetDatabaseStats()
	if err != nil {
		t.Fatalf("获取统计失败: %v", err)
	}

	if err := InsertCPUMetrics(50); err != nil {
		t.Fatalf("插入失败: %v", err)
	}

	after, _ := GetDatabaseStats()
	if after.TotalRecords != before.TotalRecords+1 {
		t.Errorf("计数器应+1: 之前 %d, 之后 %d", before.TotalRecords, after.TotalRecords)
	}
}
