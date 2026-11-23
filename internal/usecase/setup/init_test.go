package setup

import (
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestInit(t *testing.T) {
	t.Run("偽物の返した値が結果になる", func(t *testing.T) {
		// Arrange
		u := Init{
			Dir:    usecasetest.Dir{DirPath: "/cfg", Created: true},
			Policy: usecasetest.Policy{FilePath: "/cfg/policy.toml", Created: false},
		}

		// Act
		got, err := u.Run()

		// Assert
		want := InitResult{ConfigDir: "/cfg", DirCreated: true, PolicyPath: "/cfg/policy.toml", PolicyCreated: false}
		if err != nil || got != want {
			t.Errorf("got %+v, %v; want %+v", got, err, want)
		}
	})
}
