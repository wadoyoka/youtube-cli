package winpath

import "testing"

func TestFromMSYS(t *testing.T) {
	t.Run("/c の形だけ C:/ の形に直す", func(t *testing.T) {
		// Arrange
		cases := map[string]string{
			`/c/Users/me/x.json`: `C:/Users/me/x.json`,
			`/d`:                 `D:/`,
			`/c/`:                `C:/`,
			`/cc/x`:              `/cc/x`,
			`/1/x`:               `/1/x`,
			`C:\x`:               `C:\x`,
			`relative/x`:         `relative/x`,
			`/`:                  `/`,
		}
		for in, want := range cases {
			// Act
			got := FromMSYS(in)

			// Assert
			if got != want {
				t.Errorf("%q: got %q; want %q", in, got, want)
			}
		}
	})
}
