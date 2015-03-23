package capnslog

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

var pid = os.Getpid()

type Formatter interface {
	Format(pkg string, level LogLevel, depth int, entries ...LogEntry)
}

func NewStringFormatter(w io.Writer) *StringFormatter {
	return &StringFormatter{
		w: bufio.NewWriter(w),
	}
}

type StringFormatter struct {
	w *bufio.Writer
}

func (s *StringFormatter) Format(pkg string, _ LogLevel, _ int, entries ...LogEntry) {
	s.w.WriteString(pkg)
	endsInNL := false
	for _, v := range entries {
		s.w.WriteByte(' ')
		str := v.LogString()
		endsInNL = strings.HasSuffix(str, "\n")
		s.w.WriteString(str)
	}
	if !endsInNL {
		s.w.WriteString("\n")
	}
	s.w.Flush()
}

type GlogFormatter struct {
	StringFormatter
}

func NewGlogFormatter(w io.Writer) *GlogFormatter {
	g := &GlogFormatter{}
	g.w = bufio.NewWriter(w)
	return g
}

func (g GlogFormatter) Format(pkg string, level LogLevel, depth int, entries ...LogEntry) {
	g.w.Write(GlogHeader(level, depth+1))
	g.StringFormatter.Format(pkg, level, depth+1, entries...)
}

func GlogHeader(level LogLevel, depth int) []byte {
	// Lmmdd hh:mm:ss.uuuuuu threadid file:line]
	now := time.Now()
	_, file, line, ok := runtime.Caller(depth) // It's always the same number of frames to the user's call.
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	if line < 0 {
		line = 0 // not a real line number
	}
	buf := &bytes.Buffer{}
	buf.Grow(30)
	_, month, day := now.Date()
	hour, minute, second := now.Clock()
	buf.WriteString(level.Char())
	twoDigits(buf, int(month))
	twoDigits(buf, day)
	buf.WriteByte(' ')
	twoDigits(buf, hour)
	buf.WriteByte(':')
	twoDigits(buf, minute)
	buf.WriteByte(':')
	twoDigits(buf, second)
	buf.WriteByte('.')
	buf.WriteString(fmt.Sprint(now.Nanosecond() / 1000))
	buf.WriteByte(' ')
	buf.WriteString(fmt.Sprint(pid))
	buf.WriteByte(' ')
	buf.WriteString(file)
	buf.WriteByte(':')
	buf.WriteString(fmt.Sprint(line))
	buf.WriteByte(']')
	buf.WriteByte(' ')
	return buf.Bytes()
}

const digits = "0123456789"

func twoDigits(b *bytes.Buffer, d int) {
	c2 := digits[d%10]
	d /= 10
	c1 := digits[d%10]
	b.WriteByte(c1)
	b.WriteByte(c2)
}
