package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestSecretBox(t *testing.T) {
	plain := []byte("sample-secret-value")

	t.Run("暗号化して戻せる", func(t *testing.T) {
		// Arrange
		k, err := newKey()
		if err != nil {
			t.Fatal(err)
		}
		// Act
		sealed, err := sealSecret(k, secretKindToken, plain)
		if err != nil {
			t.Fatal(err)
		}
		got, err := openSecret(k, secretKindToken, sealed)
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("1バイト書き換えると読めない", func(t *testing.T) {
		// Arrange
		k, _ := newKey()
		sealed, _ := sealSecret(k, secretKindToken, plain)
		var m map[string]any
		if err := json.Unmarshal(sealed, &m); err != nil {
			t.Fatal(err)
		}
		d := []byte(m["data"].(string))
		if d[0] == 'A' {
			d[0] = 'B'
		} else {
			d[0] = 'A'
		}
		m["data"] = string(d)
		tampered, _ := json.Marshal(m)
		// Act
		_, err := openSecret(k, secretKindToken, tampered)
		// Assert
		if !errors.Is(err, errSecretMismatch) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("種類が違うと読めない", func(t *testing.T) {
		// Arrange
		k, _ := newKey()
		sealed, _ := sealSecret(k, secretKindToken, plain)
		// Act
		_, err := openSecret(k, secretKindClientSecret, sealed)
		// Assert
		if !errors.Is(err, errSecretMismatch) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("違う鍵では読めない", func(t *testing.T) {
		// Arrange
		k1, _ := newKey()
		k2, _ := newKey()
		sealed, _ := sealSecret(k1, secretKindToken, plain)
		// Act
		_, err := openSecret(k2, secretKindToken, sealed)
		// Assert
		if !errors.Is(err, errSecretMismatch) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("同じ内容でも毎回ノンスが違う", func(t *testing.T) {
		// Arrange
		k, _ := newKey()
		// Act
		a, _ := sealSecret(k, secretKindToken, plain)
		b, _ := sealSecret(k, secretKindToken, plain)
		// Assert
		if bytes.Equal(a, b) {
			t.Fatal("同じ出力になった")
		}
	})

	t.Run("形が壊れていると読めない", func(t *testing.T) {
		k, _ := newKey()
		for _, in := range []string{`{}`, `not json`, `{"v":2,"nonce":"AAAA","data":"AAAA"}`} {
			// Arrange
			sealed := []byte(in)
			// Act
			_, err := openSecret(k, secretKindToken, sealed)
			// Assert
			if !errors.Is(err, errSecretMismatch) {
				t.Fatalf("%s: err = %v", in, err)
			}
		}
	})

	t.Run("暗号文に平文が含まれない", func(t *testing.T) {
		// Arrange
		k, _ := newKey()
		// Act
		sealed, _ := sealSecret(k, secretKindToken, plain)
		// Assert
		if bytes.Contains(sealed, plain) {
			t.Fatal("平文が含まれている")
		}
	})
}
