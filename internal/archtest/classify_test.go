package archtest

import "testing"

const modulePath = "github.com/wadoyoka/youtube-cli"

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
		want    string
	}{
		{"層の中のサブパッケージも、その層にする", modulePath + "/internal/usecase/sub", "usecase"},
		{"層のパッケージから、層を読む", modulePath + "/internal/repository", "repository"},
		{"internal/archtest は LayerOther にする", modulePath + "/internal/archtest", LayerOther},
		{"internal の直下で層の外のものは空にする", modulePath + "/internal/misc", ""},
		{"cmd 以下（main）は LayerOther にする", modulePath + "/cmd/youtube-cli", LayerOther},
		{"module の外のパッケージは LayerOther にする", "fmt", LayerOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			pkgPath := tt.pkgPath

			// Act
			got := Classify(modulePath, pkgPath)

			// Assert
			if got != tt.want {
				t.Errorf("Classify(%q) = %q, 期待は %q", pkgPath, got, tt.want)
			}
		})
	}
}
