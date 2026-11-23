package cli

import (
	"fmt"
	"io"
	"sync"
)

// progressPrinter は、送信の進み具合を「\r<パス>  <割合>%」の 1 行で書き換えて表示する。
// 100% になった行は改行して残す。標準エラーがターミナルのときだけ使う（行を消すのに ANSI の制御文字を使う）。
// どの goroutine から呼ばれても行が混ざらないよう、書き込みは 1 つずつ順番に行う。
type progressPrinter struct {
	mu      sync.Mutex
	w       io.Writer
	percent map[string]int // パスごとに最後に表示した割合
	open    bool           // 改行していない行があるか
}

func newProgressPrinter(w io.Writer) *progressPrinter {
	return &progressPrinter{w: w, percent: map[string]int{}}
}

// Report は、path の進み具合（total バイトのうち done バイト）を表示する。
func (p *progressPrinter) Report(path string, done, total int64) {
	pct := 100
	if total > 0 {
		pct = int(min(max(done, 0), total) * 100 / total)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if last, ok := p.percent[path]; ok && last == pct {
		return
	}
	p.percent[path] = pct
	_, _ = fmt.Fprintf(p.w, "\r\x1b[K%s  %d%%", path, pct)
	p.open = pct < 100
	if !p.open {
		_, _ = fmt.Fprint(p.w, "\n")
	}
}

// Finish は、終わっていない行を消す。nil なら何もしない。
func (p *progressPrinter) Finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open {
		_, _ = fmt.Fprint(p.w, "\r\x1b[K")
		p.open = false
	}
}
