package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wadoyoka/youtube-cli/internal/domain"
)

func TestLockFile(t *testing.T) {
	t.Run("1つ目を持っている間は2つ目がエラーになり、解放すると取れる", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		first, err := NewLockFile(dir).Lock(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		// Act
		_, errBusy := NewLockFile(dir).Lock(ctx)
		first()
		second, errFree := NewLockFile(dir).Lock(context.Background())

		// Assert
		if domain.KindOf(errBusy) != domain.KindUnexpected || errBusy == nil {
			t.Errorf("errBusy = %v", errBusy)
		}
		if !errors.Is(errBusy, context.DeadlineExceeded) {
			t.Errorf("errBusy が時間切れではない: %v", errBusy)
		}
		if errFree != nil {
			t.Fatalf("errFree = %v", errFree)
		}
		second()
	})

	t.Run("同じ LockFile の値でも、持っている間は2つ目を取れない", func(t *testing.T) {
		// Arrange
		lock := NewLockFile(t.TempDir())
		first, err := lock.Lock(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer first()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		// Act
		_, errBusy := lock.Lock(ctx)

		// Assert
		if errBusy == nil || domain.KindOf(errBusy) != domain.KindUnexpected {
			t.Errorf("errBusy = %v", errBusy)
		}
		if !errors.Is(errBusy, context.DeadlineExceeded) {
			t.Errorf("errBusy が時間切れではない: %v", errBusy)
		}
	})
}
