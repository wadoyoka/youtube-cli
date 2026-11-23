package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	secretKindToken        = "token"
	secretKindClientSecret = "client_secret"
)

const secretBoxVersion = 1

// errSecretMismatch は、鍵か種類が違う、書き換えられた、形が違う、のどれか。
var errSecretMismatch = errors.New("暗号化したファイルを、今の鍵で読めません")

type sealedSecret struct {
	V     int    `json:"v"`
	Nonce string `json:"nonce"`
	Data  string `json:"data"`
}

// newKey は、32 バイトの乱数の鍵を返す。
func newKey() ([]byte, error) {
	key := make([]byte, keyLength)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("鍵を作れません: %w", err)
	}
	return key, nil
}

func secretAAD(kind string) []byte { return []byte("youtube-cli/" + kind + "/v1") }

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// sealSecret は、AES-256-GCM で暗号化する。AAD に種類を入れ、ノンスは毎回乱数にする。
func sealSecret(key []byte, kind string, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, fmt.Errorf("暗号化の準備ができません: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("ノンスを作れません: %w", err)
	}
	data := gcm.Seal(nil, nonce, plaintext, secretAAD(kind))
	return json.Marshal(sealedSecret{
		V:     secretBoxVersion,
		Nonce: base64.StdEncoding.EncodeToString(nonce),
		Data:  base64.StdEncoding.EncodeToString(data),
	})
}

// openSecret は sealSecret の逆。失敗はすべて errSecretMismatch で包む。
func openSecret(key []byte, kind string, sealed []byte) ([]byte, error) {
	var s sealedSecret
	if err := json.Unmarshal(sealed, &s); err != nil || s.V != secretBoxVersion {
		return nil, errSecretMismatch
	}
	nonce, err := base64.StdEncoding.DecodeString(s.Nonce)
	if err != nil {
		return nil, errSecretMismatch
	}
	data, err := base64.StdEncoding.DecodeString(s.Data)
	if err != nil {
		return nil, errSecretMismatch
	}
	gcm, err := newGCM(key)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errSecretMismatch
	}
	plain, err := gcm.Open(nil, nonce, data, secretAAD(kind))
	if err != nil {
		return nil, errSecretMismatch
	}
	return plain, nil
}
