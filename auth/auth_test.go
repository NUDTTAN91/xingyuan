package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// newTestAuthManager 构造测试用 AuthManager（固定密钥、明文密码）
func newTestAuthManager(t *testing.T, password string) *AuthManager {
	t.Helper()
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", password)
	t.Setenv("JWT_SECRET", "unit-test-secret")
	t.Setenv("MAX_LOGIN_ATTEMPTS", "3")
	t.Setenv("LOGIN_LOCK_MINUTES", "15")
	return NewAuthManager()
}

// TestGenerateAndValidateToken Token 签发与校验闭环
func TestGenerateAndValidateToken(t *testing.T) {
	am := newTestAuthManager(t, "test-password")

	access, refresh, err := am.GenerateTokenPair("admin", false)
	if err != nil {
		t.Fatalf("生成Token失败: %v", err)
	}

	// Access Token 按 access 类型校验通过
	claims, err := am.ValidateToken(access, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Access Token 校验失败: %v", err)
	}
	if claims.Username != "admin" {
		t.Errorf("用户名错误: %s", claims.Username)
	}

	// Refresh Token 按 refresh 类型校验通过
	if _, err := am.ValidateToken(refresh, TokenTypeRefresh); err != nil {
		t.Fatalf("Refresh Token 校验失败: %v", err)
	}

	// 类型交叉校验必须失败（用 refresh 冒充 access）
	if _, err := am.ValidateToken(refresh, TokenTypeAccess); err == nil {
		t.Error("Refresh Token 冒充 Access Token 应校验失败")
	}

	// 伪造/篡改的 Token 必须失败
	if _, err := am.ValidateToken(access+"x", TokenTypeAccess); err == nil {
		t.Error("篡改的 Token 应校验失败")
	}
}

// TestRevokeToken 登出黑名单：撤销后的 Token 不可再用
func TestRevokeToken(t *testing.T) {
	am := newTestAuthManager(t, "test-password")

	access, _, _ := am.GenerateTokenPair("admin", false)

	if _, err := am.ValidateToken(access, TokenTypeAccess); err != nil {
		t.Fatalf("撤销前校验应通过: %v", err)
	}
	if err := am.RevokeToken(access); err != nil {
		t.Fatalf("撤销Token失败: %v", err)
	}
	if _, err := am.ValidateToken(access, TokenTypeAccess); err == nil {
		t.Error("撤销后的 Token 应校验失败")
	}
}

// TestAuthenticate_Plaintext 明文密码认证
func TestAuthenticate_Plaintext(t *testing.T) {
	am := newTestAuthManager(t, "correct-pass")

	if err := am.Authenticate("admin", "correct-pass", "1.1.1.1"); err != nil {
		t.Errorf("正确密码应通过: %v", err)
	}
	if err := am.Authenticate("admin", "wrong-pass", "1.1.1.2"); err != ErrInvalidCredentials {
		t.Errorf("错误密码应返回 ErrInvalidCredentials, 实际: %v", err)
	}
	if err := am.Authenticate("nobody", "correct-pass", "1.1.1.3"); err != ErrInvalidCredentials {
		t.Errorf("错误用户名应返回 ErrInvalidCredentials, 实际: %v", err)
	}
}

// TestAuthenticate_Bcrypt bcrypt哈希凭据：真实密码可登录，哈希串本身不可登录
func TestAuthenticate_Bcrypt(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("real-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成bcrypt哈希失败: %v", err)
	}
	am := newTestAuthManager(t, string(hash))

	if err := am.Authenticate("admin", "real-password", "2.2.2.1"); err != nil {
		t.Errorf("真实密码应通过: %v", err)
	}
	if err := am.Authenticate("admin", string(hash), "2.2.2.2"); err != ErrInvalidCredentials {
		t.Errorf("哈希串本身登录应被拒绝, 实际: %v", err)
	}
}

// TestAuthenticate_Lockout 登录失败锁定：连续失败达到上限后锁定该IP
func TestAuthenticate_Lockout(t *testing.T) {
	am := newTestAuthManager(t, "correct-pass") // MAX_LOGIN_ATTEMPTS=3

	ip := "3.3.3.3"
	for i := 0; i < 3; i++ {
		if err := am.Authenticate("admin", "wrong", ip); err != ErrInvalidCredentials {
			t.Fatalf("第%d次失败应返回 ErrInvalidCredentials: %v", i+1, err)
		}
	}

	// 第4次：即使密码正确也应被锁定
	if err := am.Authenticate("admin", "correct-pass", ip); err != ErrTooManyAttempts {
		t.Errorf("锁定后应返回 ErrTooManyAttempts, 实际: %v", err)
	}

	// 其他IP不受影响
	if err := am.Authenticate("admin", "correct-pass", "3.3.3.4"); err != nil {
		t.Errorf("其他IP不应被锁定: %v", err)
	}
}

// TestAuthenticate_ClearOnSuccess 登录成功后清除失败计数
func TestAuthenticate_ClearOnSuccess(t *testing.T) {
	am := newTestAuthManager(t, "correct-pass")

	ip := "4.4.4.4"
	am.Authenticate("admin", "wrong", ip)
	am.Authenticate("admin", "wrong", ip)
	if err := am.Authenticate("admin", "correct-pass", ip); err != nil {
		t.Fatalf("未达锁定上限时正确密码应通过: %v", err)
	}

	// 成功后计数清零：再失败2次不应触发锁定（上限3）
	am.Authenticate("admin", "wrong", ip)
	am.Authenticate("admin", "wrong", ip)
	if err := am.Authenticate("admin", "correct-pass", ip); err != nil {
		t.Errorf("失败计数应已清零, 实际: %v", err)
	}
}
