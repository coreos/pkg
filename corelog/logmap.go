package corelog

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// LogLevel is the set of all log levels.
type LogLevel int8

const (
	// CRITICAL is the lowest log level; only errors which will end the program will be propogated.
	CRITICAL LogLevel = -1
	// ERROR is for errors that are not fatal but lead to troubling behavior.
	ERROR = 0
	// WARNING is for errors which are not fatal and not errors, but are unusual. Often sourced from misconfigurations.
	WARNING = 1
	// INFO is a log level for common, everyday log updates.
	INFO = 2
	// DEBUG is the default hidden level for more verbose updates about internal processes.
	DEBUG = 3
	// VERBOSE is for (potentially) call by call tracing of programs.
	VERBOSE = 4
)

// Char returns a single-character representation of the log level.
func (l LogLevel) Char() string {
	switch l {
	case CRITICAL:
		return "C"
	case ERROR:
		return "E"
	case WARNING:
		return "W"
	case INFO:
		return "I"
	case DEBUG:
		return "D"
	case VERBOSE:
		return "V"
	default:
		panic("Unhandled loglevel")
	}
}

// ParseLevel translates some potential loglevel strings into their corresponding levels.
func ParseLevel(s string) LogLevel {
	switch s {
	case "ERROR", "0", "E":
		return ERROR
	case "WARNING", "1", "W":
		return WARNING
	case "INFO", "2", "I":
		return INFO
	case "DEBUG", "3", "D":
		return DEBUG
	case "VERBOSE", "4", "V":
		return VERBOSE
	}
	return CRITICAL
}

type repoLogger map[string]*packageLogger

// LogEntry is the generic interface for things which can be logged.
// Implementing the single method LogString() on your objects allows you to
// format them for logs/debugging as necessary.
type LogEntry interface {
	LogString() string
}

type loggerStruct struct {
	lock      sync.Mutex
	repoMap   map[string]repoLogger
	formatter Formatter
	output    io.Writer
}

// logger is the global logger
var logger = new(loggerStruct)

// RepoLogger may return the handle to the repository's set of packages' loggers.
func RepoLogger(repo string) (repoLogger, error) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	r, ok := logger.repoMap[repo]
	if !ok {
		return nil, fmt.Errorf("No packages registered for repo %s", repo)
	}
	return r, nil
}

// MustRepoLogger returns the handle to the repository's packages' loggers.
func MustRepoLogger(repo string) repoLogger {
	r, err := RepoLogger(repo)
	if err != nil {
		panic(err)
	}
	return r
}

// SetLogLevel sets the log level for all packages in the repository.
func (r repoLogger) SetLogLevel(l LogLevel) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	for _, v := range r {
		v.level = l
	}
}

// ConfigLogLevel parses a comma-separated string of "package=loglevel", in
// order, and sets the log levels in each package appropriately.
func (r repoLogger) ConfigLogLevel(conf string) {
	setlist := strings.Split(conf, ",")
	logger.lock.Lock()
	defer logger.lock.Unlock()
	for _, setstring := range setlist {
		setting := strings.Split(setstring, "=")
		if len(setting) != 2 {
			continue
		}
		if setting[0] == "*" {
			l := ParseLevel(setting[1])
			for _, v := range r {
				v.level = l
			}
			continue
		}
		l, ok := r[setting[0]]
		if !ok {
			continue
		}
		l.level = ParseLevel(setting[1])
	}

}

// SetOutput sets the output io.Writer of all logs.
func SetOutput(output io.Writer) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	logger.output = output
	if logger.formatter != nil {
		logger.formatter.SetWriter(logger.output)
	}
}

// SetOutput sets the formatting function for all logs.
func SetFormatter(f Formatter) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	logger.formatter = f
	if logger.output != nil {
		logger.formatter.SetWriter(logger.output)
	}
}

// NewPackageLogger creates a package logger object.
// This should be defined as a global var in your package, referencing your repo.
func NewPackageLogger(repo string, pkg string) (p *packageLogger) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	if logger.repoMap == nil {
		logger.repoMap = make(map[string]repoLogger)
	}
	r, rok := logger.repoMap[repo]
	if !rok {
		logger.repoMap[repo] = make(repoLogger)
		r = logger.repoMap[repo]
	}
	p, pok := r[pkg]
	if !pok {
		r[pkg] = &packageLogger{
			pkg:   pkg,
			level: CRITICAL,
		}
		p = r[pkg]
	}
	return
}

type BaseLogEntry string

func (b BaseLogEntry) LogString() string {
	return string(b)
}
