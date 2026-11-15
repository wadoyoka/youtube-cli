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
			"YouTube":       (*usecasetest.YouTube)(nil),
			"StateStore":    (*usecasetest.State)(nil),
			"Locker":        (*usecasetest.Locker)(nil),
			"Ledger":        (*usecasetest.Ledger)(nil),
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
			case "YouTube":
				_, ok = fake.(usecase.YouTube)
			case "StateStore":
				_, ok = fake.(usecase.StateStore)
			case "Locker":
				_, ok = fake.(usecase.Locker)
			case "Ledger":
				_, ok = fake.(usecase.Ledger)
			}
			if !ok {
				t.Errorf("%s の偽物がポートを満たしません", name)
			}
		}
	})
}
