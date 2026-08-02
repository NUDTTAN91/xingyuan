package remote

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// encPrefix 加密密文的标识前缀，用于区分存量明文与密文
const encPrefix = "enc:v1:"

// loadOrCreateEncryptKey 加载或生成密码加密密钥
// 密钥持久化在数据目录（随 ./data 卷保留），保证重启后仍能解密
func loadOrCreateEncryptKey(dataDir string) ([]byte, error) {
	keyPath := filepath.Join(dataDir, ".encrypt_key")

	// 尝试读取已有密钥
	if data, err := os.ReadFile(keyPath); err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err == nil && len(key) == 32 {
			return key, nil
		}
		return nil, fmt.Errorf("密钥文件 %s 内容无效", keyPath)
	}

	// 首次运行：生成32字节随机密钥并落盘
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成加密密钥失败: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, fmt.Errorf("保存加密密钥失败: %v", err)
	}
	log.Printf("已生成远程主机密码加密密钥: %s", keyPath)
	return key, nil
}

// encryptPassword 使用 AES-256-GCM 加密密码
func (m *Manager) encryptPassword(plain string) (string, error) {
	block, err := aes.NewCipher(m.encryptKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptPassword 解密密码；对无前缀的存量明文原样返回（兼容旧数据）
func (m *Manager) decryptPassword(stored string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encPrefix))
	if err != nil {
		return "", fmt.Errorf("密文格式无效: %v", err)
	}
	block, err := aes.NewCipher(m.encryptKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("密文长度无效")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("解密失败（加密密钥可能已变更）: %v", err)
	}
	return string(plain), nil
}

// migratePlaintextPasswords 启动时将存量明文密码加密（一次性迁移）
func (m *Manager) migratePlaintextPasswords() {
	rows, err := m.db.Query("SELECT id, password FROM remote_hosts")
	if err != nil {
		log.Printf("密码迁移: 查询主机失败: %v", err)
		return
	}

	type hostPw struct {
		id       int
		password string
	}
	var pending []hostPw
	for rows.Next() {
		var h hostPw
		if err := rows.Scan(&h.id, &h.password); err != nil {
			log.Printf("密码迁移: 读取主机记录失败: %v", err)
			continue
		}
		if !strings.HasPrefix(h.password, encPrefix) {
			pending = append(pending, h)
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("密码迁移: 遍历主机记录失败: %v", err)
	}
	rows.Close()

	if len(pending) == 0 {
		return
	}

	migrated := 0
	for _, h := range pending {
		encrypted, err := m.encryptPassword(h.password)
		if err != nil {
			log.Printf("密码迁移: 加密主机ID=%d失败: %v", h.id, err)
			continue
		}
		if _, err := m.db.Exec("UPDATE remote_hosts SET password = ? WHERE id = ?", encrypted, h.id); err != nil {
			log.Printf("密码迁移: 更新主机ID=%d失败: %v", h.id, err)
			continue
		}
		migrated++
	}
	log.Printf("密码迁移完成: %d 台远程主机的明文密码已加密存储", migrated)
}
