package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorKind(t *testing.T) {
	t.Run("各種別の終了コードと名前が決まっている", func(t *testing.T) {
		// Arrange
		cases := []struct {
			kind ErrorKind
			code int
			name string
		}{
			{KindUnexpected, 1, "unexpected"},
			{KindUsage, 2, "usage"},
			{KindSetup, 3, "setup"},
			{KindNotFound, 4, "not_found"},
			{KindConflict, 5, "conflict"},
			{KindAmbiguous, 6, "ambiguous"},
			{KindDenied, 7, "denied"},
		}

		for _, c := range cases {
			// Act
			code, name := c.kind.ExitCode(), c.kind.String()

			// Assert
			if code != c.code || name != c.name {
				t.Errorf("%v: got (%d, %q), want (%d, %q)", c.kind, code, name, c.code, c.name)
			}
		}
	})

	t.Run("包まれていても種別を取り出せる", func(t *testing.T) {
		// Arrange
		err := fmt.Errorf("x: %w", Errorf(KindConflict, "c"))

		// Act
		got := KindOf(err)

		// Assert
		if got != KindConflict {
			t.Errorf("got %v, want %v", got, KindConflict)
		}
	})

	t.Run("domain のエラーでなければ予期しないエラーになる", func(t *testing.T) {
		// Arrange
		err := errors.New("x")

		// Act
		got := KindOf(err)

		// Assert
		if got != KindUnexpected {
			t.Errorf("got %v, want %v", got, KindUnexpected)
		}
	})

	t.Run("Wrap した原因が文言に付き Unwrap できる", func(t *testing.T) {
		// Arrange
		cause := errors.New("原因")

		// Act
		err := Errorf(KindSetup, "準備: %d", 1).WithDetails("a", "b").Wrap(cause)

		// Assert
		if err.Error() != "準備: 1: 原因" {
			t.Errorf("Error() = %q", err.Error())
		}
		if !errors.Is(err, cause) {
			t.Error("errors.Is が原因を見つけられない")
		}
		if len(err.Details) != 2 {
			t.Errorf("Details = %v", err.Details)
		}
	})

	t.Run("原因がなければ Message だけを返す", func(t *testing.T) {
		// Arrange
		err := Errorf(KindUsage, "m")

		// Act
		got := err.Error()

		// Assert
		if got != "m" {
			t.Errorf("got %q", got)
		}
	})
}
