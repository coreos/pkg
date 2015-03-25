package capnslog

import (
	"fmt"
	"os"
)

type packageLogger struct {
	pkg   string
	level LogLevel
}

const calldepth = 3

func (p *packageLogger) internalLog(depth int, inLevel LogLevel, entries ...LogEntry) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	if logger.formatter != nil {
		logger.formatter.Format(p.pkg, inLevel, depth+1, entries...)
	}
}

func (p *packageLogger) LevelAt(l LogLevel) bool {
	return p.level >= l
}

// log stdlib compatibility
func (p *packageLogger) Println(args ...interface{}) {
	if p.level < INFO {
		return
	}
	p.internalLog(calldepth, INFO, BaseLogEntry(fmt.Sprintln(args...)))
}

func (p *packageLogger) Printf(format string, args ...interface{}) {
	if p.level < INFO {
		return
	}
	p.internalLog(calldepth, INFO, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Print(args ...interface{}) {
	if p.level < INFO {
		return
	}
	p.internalLog(calldepth, INFO, BaseLogEntry(fmt.Sprint(args...)))
}

// Panic and fatal

func (p *packageLogger) Panicf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	p.internalLog(calldepth, CRITICAL, BaseLogEntry(s))
	panic(s)
}

func (p *packageLogger) Panic(args ...interface{}) {
	s := fmt.Sprint(args...)
	p.internalLog(calldepth, CRITICAL, BaseLogEntry(s))
	panic(s)
}

func (p *packageLogger) Fatalf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	p.internalLog(calldepth, CRITICAL, BaseLogEntry(s))
	os.Exit(1)
}

func (p *packageLogger) Fatal(args ...interface{}) {
	s := fmt.Sprint(args...)
	p.internalLog(calldepth, CRITICAL, BaseLogEntry(s))
	os.Exit(1)
}

// Error Functions
func (p *packageLogger) Errorf(format string, args ...interface{}) {
	if p.level < ERROR {
		return
	}
	p.internalLog(calldepth, ERROR, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Error(entries ...LogEntry) {
	if p.level < ERROR {
		return
	}
	p.internalLog(calldepth, ERROR, entries...)
}

// Warning Functions
func (p *packageLogger) Warningf(format string, args ...interface{}) {
	if p.level < WARNING {
		return
	}
	p.internalLog(calldepth, WARNING, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Warning(entries ...LogEntry) {
	if p.level < WARNING {
		return
	}
	p.internalLog(calldepth, WARNING, entries...)
}

// Notice Functions
func (p *packageLogger) Noticef(format string, args ...interface{}) {
	if p.level < NOTICE {
		return
	}
	p.internalLog(calldepth, NOTICE, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Notice(entries ...LogEntry) {
	if p.level < NOTICE {
		return
	}
	p.internalLog(calldepth, NOTICE, entries...)
}

// Info Functions
func (p *packageLogger) Infof(format string, args ...interface{}) {
	if p.level < INFO {
		return
	}
	p.internalLog(calldepth, INFO, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Info(entries ...LogEntry) {
	if p.level < INFO {
		return
	}
	p.internalLog(calldepth, INFO, entries...)
}

// Debug Functions
func (p *packageLogger) Debugf(format string, args ...interface{}) {
	if p.level < DEBUG {
		return
	}
	p.internalLog(calldepth, DEBUG, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Debug(entries ...LogEntry) {
	if p.level < DEBUG {
		return
	}
	p.internalLog(calldepth, DEBUG, entries...)
}

// Trace Functions
func (p *packageLogger) Tracef(format string, args ...interface{}) {
	if p.level < TRACE {
		return
	}
	p.internalLog(calldepth, TRACE, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Trace(entries ...LogEntry) {
	if p.level < TRACE {
		return
	}
	p.internalLog(calldepth, TRACE, entries...)
}
