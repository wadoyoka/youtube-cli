package terminal

import (
	"encoding/binary"
	"os"

	"golang.org/x/sys/windows"
)

// isCygwinPTY は、f が Cygwin・MSYS の疑似端末（mintty）の名前付きパイプかを返す。読めなければ false。
func isCygwinPTY(f *os.File) bool {
	h := windows.Handle(f.Fd())
	if t, err := windows.GetFileType(h); err != nil || t != windows.FILE_TYPE_PIPE {
		return false
	}
	// FILE_NAME_INFO: 先頭 4 バイトが名前のバイト数、続きが UTF-16 の名前。
	const bufSize = 4 + 2*windows.MAX_PATH
	buf := make([]byte, bufSize)
	if err := windows.GetFileInformationByHandleEx(h, windows.FileNameInfo, &buf[0], bufSize); err != nil {
		return false
	}
	n := int(binary.LittleEndian.Uint32(buf))
	if n < 0 || n > len(buf)-4 || n%2 != 0 {
		return false
	}
	u := make([]uint16, n/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(buf[4+2*i:])
	}
	return isCygwinPipeName(windows.UTF16ToString(u))
}

// EnableVT は、f のコンソールに仮想端末の処理を付けられたら true を返す。
// コンソールでなく mintty なら、mintty が自分で解釈するので true。それ以外は false。
func EnableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return isCygwinPTY(f)
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
