package collector

import "testing"

// TestUpdateBaseline_NormalGrowth 正常增长：基准值不变，累计值 = 基准 + 当前
func TestUpdateBaseline_NormalGrowth(t *testing.T) {
	b := &NetworkBaseline{BytesRecvBaseline: 1000, BytesSentBaseline: 500, LastRecv: 100, LastSent: 50}

	recv, sent := updateBaseline(b, 200, 80)

	if recv != 1200 || sent != 580 {
		t.Errorf("累计值错误: recv=%d(期望1200), sent=%d(期望580)", recv, sent)
	}
	if b.BytesRecvBaseline != 1000 || b.BytesSentBaseline != 500 {
		t.Errorf("正常增长时基准值不应变化: recvBase=%d, sentBase=%d", b.BytesRecvBaseline, b.BytesSentBaseline)
	}
	if b.LastRecv != 200 || b.LastSent != 80 {
		t.Errorf("Last值未更新: lastRecv=%d, lastSent=%d", b.LastRecv, b.LastSent)
	}
}

// TestUpdateBaseline_CounterWrap 计数器回绕（宿主机重启）：上次累计滚入基准
func TestUpdateBaseline_CounterWrap(t *testing.T) {
	b := &NetworkBaseline{BytesRecvBaseline: 1000, BytesSentBaseline: 500, LastRecv: 300, LastSent: 200}

	// 当前值小于上次值 → 判定重启
	recv, sent := updateBaseline(b, 10, 5)

	if b.BytesRecvBaseline != 1300 || b.BytesSentBaseline != 700 {
		t.Errorf("回绕后基准值错误: recvBase=%d(期望1300), sentBase=%d(期望700)", b.BytesRecvBaseline, b.BytesSentBaseline)
	}
	if recv != 1310 || sent != 705 {
		t.Errorf("回绕后累计值错误: recv=%d(期望1310), sent=%d(期望705)", recv, sent)
	}
}

// TestUpdateBaseline_FirstCollect 首次采集（LastRecv=0）：不触发回绕判定
func TestUpdateBaseline_FirstCollect(t *testing.T) {
	b := &NetworkBaseline{}

	recv, sent := updateBaseline(b, 100, 50)

	if b.BytesRecvBaseline != 0 || b.BytesSentBaseline != 0 {
		t.Errorf("首次采集不应改变基准值: recvBase=%d, sentBase=%d", b.BytesRecvBaseline, b.BytesSentBaseline)
	}
	if recv != 100 || sent != 50 {
		t.Errorf("首次采集累计值错误: recv=%d, sent=%d", recv, sent)
	}
}

// TestUpdateBaseline_ConsecutiveWraps 连续两次重启：基准应累计叠加
func TestUpdateBaseline_ConsecutiveWraps(t *testing.T) {
	b := &NetworkBaseline{}

	updateBaseline(b, 100, 100) // 首次
	updateBaseline(b, 10, 10)   // 第一次回绕: 基准 +100
	updateBaseline(b, 50, 50)   // 正常增长
	recv, sent := updateBaseline(b, 5, 5) // 第二次回绕: 基准再 +50

	if b.BytesRecvBaseline != 150 || b.BytesSentBaseline != 150 {
		t.Errorf("连续回绕基准值错误: recvBase=%d(期望150), sentBase=%d(期望150)", b.BytesRecvBaseline, b.BytesSentBaseline)
	}
	if recv != 155 || sent != 155 {
		t.Errorf("连续回绕累计值错误: recv=%d(期望155), sent=%d(期望155)", recv, sent)
	}
}
