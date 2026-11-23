// Package terminal は、標準入出力が端末かどうかの判定と、端末の表示の設定を持つ。
package terminal

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// IsTerminal は、f が端末かどうかを返す。
// Windows では、Git Bash の mintty が使う疑似端末（名前付きパイプ）も端末とみなす。
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd())) || isCygwinPTY(f)
}

// isCygwinPipeName は、名前付きパイプの名前が Cygwin・MSYS の疑似端末のものかを返す。
// 形は `\msys-<id>-pty<N>-(from|to)-master`（mattn/go-isatty と同じ）。OS は見ない。
func isCygwinPipeName(name string) bool {
	parts := strings.Split(name, "-")
	if len(parts) < 5 {
		return false
	}
	switch parts[0] {
	case `\msys`, `\cygwin`, `\Device\NamedPipe\msys`, `\Device\NamedPipe\cygwin`:
	default:
		return false
	}
	return parts[1] != "" &&
		strings.HasPrefix(parts[2], "pty") &&
		(parts[3] == "from" || parts[3] == "to") &&
		parts[4] == "master"
}
