package cli

import (
	"fmt"
	"io"
)

// printer は複数回の書き込みのうち最初の書き込みエラーを err に残し、エラーの後は書き込まない。
// パイプの読み手が先に終了したときなどに、出力が途中で切れたことを終了コードで示すため。
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, a ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, a...)
	}
}

func (p *printer) println(a ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintln(p.w, a...)
	}
}
