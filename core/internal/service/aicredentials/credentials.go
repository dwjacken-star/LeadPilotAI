package aicredentials

import (
	"billionmail-core/internal/service/public"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
)

func StorageRoot() string {
	if root := os.Getenv("MAPLEMAIL_GROWTH_DATA"); root != "" {
		return root
	}
	return public.AbsPath("../core/data/growth-files")
}
func encryptionKey() ([]byte, error) {
	if e := os.MkdirAll(StorageRoot(), 0700); e != nil {
		return nil, e
	}
	path := filepath.Join(StorageRoot(), ".ai-master-key")
	b, e := os.ReadFile(path)
	if e == nil {
		if len(b) != 32 {
			return nil, errors.New("invalid AI encryption key")
		}
		return b, nil
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	b = make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(e) {
		return os.ReadFile(path)
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	_, e = f.Write(b)
	return b, e
}
func Protect(value string, decode bool) (string, error) {
	key, e := encryptionKey()
	if e != nil {
		return "", e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return "", e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	if decode {
		b, e := base64.StdEncoding.DecodeString(value)
		if e != nil || len(b) < aead.NonceSize() {
			return "", errors.New("invalid encrypted credential")
		}
		plain, e := aead.Open(nil, b[:aead.NonceSize()], b[aead.NonceSize():], []byte("growth"))
		return string(plain), e
	}
	nonce := make([]byte, aead.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(value), []byte("growth"))), nil
}
