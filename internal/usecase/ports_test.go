package usecase_test

import (
	"testing"

	"github.com/wadoyoka/youtube-cli/internal/usecase"
	"github.com/wadoyoka/youtube-cli/internal/usecase/usecasetest"
)

func TestFakesSatisfyPorts(t *testing.T) {
	t.Run("テスト用の偽物はポートを満たす", func(t *testing.T) {
		// Arrange
		ports := map[string]any{
			"Authenticator": (*usecasetest.Auth)(nil),
			"ConfigDir":     usecasetest.Dir{},
			"PolicyStore":   usecasetest.Policy{},
		}

		// Act / Assert
		for name, fake := range ports {
			var ok bool
			switch name {
			case "Authenticator":
				_, ok = fake.(usecase.Authenticator)
			case "ConfigDir":
				_, ok = fake.(usecase.ConfigDir)
			case "PolicyStore":
				_, ok = fake.(usecase.PolicyStore)
			}
			if !ok {
				t.Errorf("%s の偽物がポートを満たしません", name)
			}
		}
	})
}
