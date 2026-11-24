package archtest

import (
	"strings"
	"testing"
)

func TestCheckImport(t *testing.T) {
	tests := []struct {
		name    string
		from    string
		to      string
		wantErr bool
		wantMsg string
	}{
		{name: "usecase から repository は落とす", from: "usecase", to: "repository", wantErr: true},
		{name: "repository から usecase は許す", from: "repository", to: "usecase"},
		{name: "domain から usecase は落とす", from: "domain", to: "usecase", wantErr: true},
		{name: "cli から repository は落とし、main.go の役目だと伝える", from: "cli", to: "repository", wantErr: true, wantMsg: "main.go の役目"},
		{name: "cli から usecase は許す", from: "cli", to: "usecase"},
		{name: "usecase から domain は許す", from: "usecase", to: "domain"},
		{name: "repository から domain は許す", from: "repository", to: "domain"},
		{name: "同じ層（サブパッケージ）どうしは許す", from: "domain", to: "domain"},
		{name: "domain から cli は落とす", from: "domain", to: "cli", wantErr: true},
		{name: "LayerOther からは何でも許す", from: LayerOther, to: "repository"},
		{name: "層から LayerOther は確かめない", from: "domain", to: LayerOther},
		{name: "層の外のパッケージからは落とす", from: "", to: "domain", wantErr: true},
		{name: "層の外のパッケージへは落とす", from: "usecase", to: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			from, to := tt.from, tt.to

			// Act
			err := CheckImport(from, to)

			// Assert
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("CheckImport(%q, %q) = %v, 落とすかどうかの期待は %v", from, to, err, tt.wantErr)
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("エラーの文面に %q が入っているはずが %q でした", tt.wantMsg, err)
			}
		})
	}
}
