package browser

import (
	"reflect"
	"testing"
)

func TestCommand(t *testing.T) {
	const url = "https://accounts.example.com/o?a=1&b=2"

	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
	}{
		{
			name:     "Windows は rundll32 で開く",
			goos:     "windows",
			wantName: "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", url},
		},
		{
			name:     "macOS は open で開く",
			goos:     "darwin",
			wantName: "open",
			wantArgs: []string{url},
		},
		{
			name:     "Linux は xdg-open で開く",
			goos:     "linux",
			wantName: "xdg-open",
			wantArgs: []string{url},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			goos := tt.goos

			// Act
			gotName, gotArgs := command(goos, url)

			// Assert
			if gotName != tt.wantName {
				t.Errorf("command(%q) の名前 = %q, want %q", goos, gotName, tt.wantName)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("command(%q) の引数 = %q, want %q", goos, gotArgs, tt.wantArgs)
			}
		})
	}
}
