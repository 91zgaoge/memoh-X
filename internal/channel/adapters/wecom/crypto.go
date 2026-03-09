package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"strings"
)

// decryptFile 使用 AES-256-CBC 解密文件
//
// 参数:
//   - encryptedData: 加密的文件数据
//   - aesKey: Base64 编码的 AES-256 密钥
//
// 返回:
//   - 解密后的文件数据
//   - 错误（如果有）
//
// 注意: WeCom使用PKCS#7填充至32字节倍数，IV取aesKey解码后的前16字节
func decryptFile(encryptedData []byte, aesKey string) ([]byte, error) {
	// 参数验证
	if len(encryptedData) == 0 {
		return nil, fmt.Errorf("decryptFile: encrypted data is empty")
	}

	if aesKey == "" {
		return nil, fmt.Errorf("decryptFile: aesKey is empty")
	}

	// 清理 aesKey：移除空白字符和换行符
	aesKey = strings.TrimSpace(aesKey)

	// 尝试多种 Base64 解码方式
	var key []byte
	var err error

	// 方式1: 标准 Base64
	key, err = base64.StdEncoding.DecodeString(aesKey)
	if err != nil {
		// 方式2: URL 安全的 Base64
		key, err = base64.URLEncoding.DecodeString(aesKey)
		if err != nil {
			// 方式3: 尝试添加 padding
			padding := 4 - len(aesKey)%4
			if padding != 4 {
				aesKey += strings.Repeat("=", padding)
			}
			key, err = base64.StdEncoding.DecodeString(aesKey)
			if err != nil {
				return nil, fmt.Errorf("decryptFile: failed to decode aesKey with all methods: %w", err)
			}
		}
	}

	// 验证密钥长度（AES-256需要32字节）
	if len(key) != 32 {
		return nil, fmt.Errorf("decryptFile: invalid key length, expected 32 bytes, got %d", len(key))
	}

	// IV 取 aesKey 解码后的前 16 字节
	iv := key[:16]

	// 创建 AES 解密器
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("decryptFile: failed to create cipher: %w", err)
	}

	// 使用 CBC 模式
	mode := cipher.NewCBCDecrypter(block, iv)

	// 解密数据
	decrypted := make([]byte, len(encryptedData))
	mode.CryptBlocks(decrypted, encryptedData)

	// 手动去除 PKCS#7 填充（支持32字节 block）
	padLen := int(decrypted[len(decrypted)-1])
	if padLen < 1 || padLen > 32 || padLen > len(decrypted) {
		return nil, fmt.Errorf("decryptFile: invalid PKCS#7 padding value: %d", padLen)
	}

	// 验证所有填充字节是否一致
	for i := len(decrypted) - padLen; i < len(decrypted); i++ {
		if int(decrypted[i]) != padLen {
			return nil, fmt.Errorf("decryptFile: invalid PKCS#7 padding: padding bytes mismatch")
		}
	}

	return decrypted[:len(decrypted)-padLen], nil
}
