package atomicfile

import (
	"errors"
	"testing"
	"time"
)

func TestRenameRetrying(t *testing.T) {
	errTransient := errors.New("一時的な失敗")
	isTransient := func(err error) bool { return errors.Is(err, errTransient) }

	t.Run("一時的な失敗が2回続いたあとに成功すれば nil", func(t *testing.T) {
		// Arrange
		calls := 0
		rename := func(_, _ string) error {
			calls++
			if calls <= 2 {
				return errTransient
			}
			return nil
		}

		// Act
		err := renameRetrying(rename, isTransient, "a", "b", 5, 0)
		// Assert
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if calls != 3 {
			t.Errorf("calls = %d, want 3", calls)
		}
	})

	t.Run("一時的でない失敗はすぐ返す", func(t *testing.T) {
		// Arrange
		errOther := errors.New("別のエラー")
		calls := 0
		rename := func(_, _ string) error {
			calls++
			return errOther
		}

		// Act
		err := renameRetrying(rename, isTransient, "a", "b", 5, 0)

		// Assert
		if !errors.Is(err, errOther) {
			t.Errorf("err = %v, want %v", err, errOther)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	t.Run("回数が 0 でも rename を 1 回は呼び、その結果を返す", func(t *testing.T) {
		// Arrange
		calls := 0
		rename := func(_, _ string) error {
			calls++
			return errTransient
		}

		// Act
		err := renameRetrying(rename, isTransient, "from", "to", 0, time.Millisecond)

		// Assert
		if !errors.Is(err, errTransient) {
			t.Errorf("err = %v, want %v", err, errTransient)
		}
		if calls != 1 {
			t.Errorf("calls = %d, want 1", calls)
		}
	})

	t.Run("回数を使い切ったら最後の失敗を返す", func(t *testing.T) {
		// Arrange
		calls := 0
		rename := func(_, _ string) error {
			calls++
			return errTransient
		}

		// Act
		err := renameRetrying(rename, isTransient, "a", "b", 3, 0)

		// Assert
		if !errors.Is(err, errTransient) {
			t.Errorf("err = %v, want %v", err, errTransient)
		}
		if calls != 3 {
			t.Errorf("calls = %d, want 3", calls)
		}
	})
}
