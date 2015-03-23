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

func (p *packageLogger) Panicln(args ...interface{}) {
	s := fmt.Sprintln(args...)
	p.internalLog(calldepth, CRITICAL, BaseLogEntry(s))
	panic(s)
}

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

func (p *packageLogger) Fatalln(args ...interface{}) {
	s := fmt.Sprintln(args...)
	p.internalLog(calldepth, CRITICAL, BaseLogEntry(s))
	os.Exit(1)
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
func (p *packageLogger) Errorln(args ...interface{}) {
	if p.level < ERROR {
		return
	}
	p.internalLog(calldepth, ERROR, BaseLogEntry(fmt.Sprintln(args...)))
}

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

func (p *packageLogger) ERROR() bool {
	return p.level < ERROR
}

// Warning Functions
func (p *packageLogger) Warningln(args ...interface{}) {
	if p.level < WARNING {
		return
	}
	p.internalLog(calldepth, WARNING, BaseLogEntry(fmt.Sprintln(args...)))
}

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

func (p *packageLogger) WARNING() bool {
	return p.level < WARNING
}

// Notice Functions
func (p *packageLogger) Noticeln(args ...interface{}) {
	if p.level < NOTICE {
		return
	}
	p.internalLog(calldepth, NOTICE, BaseLogEntry(fmt.Sprintln(args...)))
}

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

func (p *packageLogger) NOTICE() bool {
	return p.level < NOTICE
}

// Info Functions
func (p *packageLogger) Infoln(args ...interface{}) {
	if p.level < INFO {
		return
	}
	p.internalLog(calldepth, INFO, BaseLogEntry(fmt.Sprintln(args...)))
}

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

func (p *packageLogger) INFO() bool {
	return p.level < INFO
}

// Debug Functions
func (p *packageLogger) Debugln(args ...interface{}) {
	if p.level < DEBUG {
		return
	}
	p.internalLog(calldepth, DEBUG, BaseLogEntry(fmt.Sprintln(args...)))
}

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

func (p *packageLogger) DEBUG() bool {
	return p.level < DEBUG
}

// Verbose Functions
func (p *packageLogger) Verboseln(args ...interface{}) {
	if p.level < VERBOSE {
		return
	}
	p.internalLog(calldepth, VERBOSE, BaseLogEntry(fmt.Sprintln(args...)))
}

func (p *packageLogger) Verbosef(format string, args ...interface{}) {
	if p.level < VERBOSE {
		return
	}
	p.internalLog(calldepth, VERBOSE, BaseLogEntry(fmt.Sprintf(format, args...)))
}

func (p *packageLogger) Verbose(entries ...LogEntry) {
	if p.level < VERBOSE {
		return
	}
	p.internalLog(calldepth, VERBOSE, entries...)
}

func (p *packageLogger) VERBOSE() bool {
	return p.level < VERBOSE
}
