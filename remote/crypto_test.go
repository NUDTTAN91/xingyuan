package remote

import (
	"strings"
	"testing"
)

// newTestManager 构造仅含加密密钥的 Manager（不依赖数据库）
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	key, err := loadOrCreateEncryptKey(t.TempDir())
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	return &Manager{encryptKey: key}
}

// TestEncryptDecryptRoundTrip 加密解密往返
func TestEncryptDecryptRoundTrip(t *testing.T) {
	m := newTestManager(t)

	for _, plain := range []string{"simple", "复杂密码!@#$%^&*()", "", "with space and 中文"} {
		encrypted, err := m.encryptPassword(plain)
		if err != nil {
			t.Fatalf("加密 %q 失败: %v", plain, err)
		}
		if !strings.HasPrefix(encrypted, encPrefix) {
			t.Errorf("密文缺少前缀: %s", encrypted)
		}
		if encrypted == plain {
			t.Errorf("密文不应等于明文")
		}

		decrypted, err := m.decryptPassword(encrypted)
		if err != nil {
			t.Fatalf("解密失败: %v", err)
		}
		if decrypted != plain {
			t.Errorf("往返结果不一致: 期望 %q, 实际 %q", plain, decrypted)
		}
	}
}

// TestDecryptLegacyPlaintext 存量明文（无前缀）应原样返回
func TestDecryptLegacyPlaintext(t *testing.T) {
	m := newTestManager(t)

	got, err := m.decryptPassword("legacy-plain-password")
	if err != nil {
		t.Fatalf("明文兼容处理失败: %v", err)
	}
	if got != "legacy-plain-password" {
		t.Errorf("明文应原样返回, 实际: %q", got)
	}
}

// TestDecryptWithWrongKey 用错误密钥解密应失败
func TestDecryptWithWrongKey(t *testing.T) {
	m1 := newTestManager(t)
	m2 := newTestManager(t) // 不同 TempDir，密钥不同

	encrypted, _ := m1.encryptPassword("secret")
	if _, err := m2.decryptPassword(encrypted); err == nil {
		t.Error("错误密钥解密应返回错误")
	}
}

// TestEncryptNonDeterministic 相同明文两次加密结果应不同（随机nonce）
func TestEncryptNonDeterministic(t *testing.T) {
	m := newTestManager(t)

	e1, _ := m.encryptPassword("same-password")
	e2, _ := m.encryptPassword("same-password")
	if e1 == e2 {
		t.Error("相同明文两次加密不应产生相同密文")
	}
}

// TestLoadOrCreateEncryptKey_Persistent 密钥落盘后重复加载应得到同一密钥
func TestLoadOrCreateEncryptKey_Persistent(t *testing.T) {
	dir := t.TempDir()

	key1, err := loadOrCreateEncryptKey(dir)
	if err != nil {
		t.Fatalf("首次生成密钥失败: %v", err)
	}
	key2, err := loadOrCreateEncryptKey(dir)
	if err != nil {
		t.Fatalf("二次加载密钥失败: %v", err)
	}
	if string(key1) != string(key2) {
		t.Error("同一目录两次加载的密钥应一致")
	}
	if len(key1) != 32 {
		t.Errorf("密钥长度应为32字节, 实际 %d", len(key1))
	}
}
