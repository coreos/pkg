package capnslog

import (
	"bufio"
	"io"
	"strings"
)

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
	if pkg != "" {
		s.w.WriteString(pkg + ":")
	}
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
