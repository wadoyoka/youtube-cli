package winpath

import "strings"

// FromMSYS は、`/c` や `/c/...` の形（mintty にファイルを落としたときの形）を `C:/...` に直す。
// 1 文字の英字のあとが終わりか `/` のものだけを直し、それ以外はそのまま返す。
func FromMSYS(s string) string {
	if len(s) < 2 || s[0] != '/' || !isASCIILetter(s[1]) {
		return s
	}
	if len(s) == 2 {
		return strings.ToUpper(s[1:2]) + ":/"
	}
	if s[2] != '/' {
		return s
	}
	return strings.ToUpper(s[1:2]) + ":" + s[2:]
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
