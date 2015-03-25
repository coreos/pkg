package capnslog

import (
	"fmt"
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
	// NOTICE is for normal but significant conditions.
	NOTICE = 2
	// INFO is a log level for common, everyday log updates.
	INFO = 3
	// DEBUG is the default hidden level for more verbose updates about internal processes.
	DEBUG = 4
	// TRACE is for (potentially) call by call tracing of programs.
	TRACE = 5
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
	case NOTICE:
		return "N"
	case INFO:
		return "I"
	case DEBUG:
		return "D"
	case TRACE:
		return "T"
	default:
		panic("Unhandled loglevel")
	}
}

// ParseLevel translates some potential loglevel strings into their corresponding levels.
func ParseLevel(s string) (LogLevel, error) {
	switch s {
	case "CRITICAL", "C":
		return CRITICAL, nil
	case "ERROR", "0", "E":
		return ERROR, nil
	case "WARNING", "1", "W":
		return WARNING, nil
	case "NOTICE", "2", "N":
		return INFO, nil
	case "INFO", "3", "I":
		return INFO, nil
	case "DEBUG", "4", "D":
		return DEBUG, nil
	case "TRACE", "5", "T":
		return TRACE, nil
	}
	return CRITICAL, fmt.Errorf("couldn't parse log level %s", s)
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
}

// logger is the global logger
var logger = new(loggerStruct)

// RepoLogger may return the handle to the repository's set of packages' loggers.
func RepoLogger(repo string) (repoLogger, error) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	r, ok := logger.repoMap[repo]
	if !ok {
		return nil, fmt.Errorf("no packages registered for repo %s", repo)
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
func (r repoLogger) SetGlobalLogLevel(l LogLevel) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	for _, v := range r {
		v.level = l
	}
}

// ParseLogLevelConfig parses a comma-separated string of "package=loglevel", in
// order, and returns a map of the results, for use in SetLogLevel.
func (r repoLogger) ParseLogLevelConfig(conf string) (map[string]LogLevel, error) {
	setlist := strings.Split(conf, ",")
	out := make(map[string]LogLevel)
	for _, setstring := range setlist {
		setting := strings.Split(setstring, "=")
		if len(setting) != 2 {
			continue
		}
		l, err := ParseLevel(setting[1])
		if err != nil {
			return nil, err
		}
		out[setting[0]] = l
	}
	return out, nil
}

func (r repoLogger) SetLogLevel(m map[string]LogLevel) {
	if l, ok := m["*"]; ok {
		r.SetGlobalLogLevel(l)
	}
	logger.lock.Lock()
	defer logger.lock.Unlock()
	for k, v := range m {
		l, ok := r[k]
		if !ok {
			continue
		}
		l.level = v
	}
}

// SetFormatter sets the formatting function for all logs.
func SetFormatter(f Formatter) {
	logger.lock.Lock()
	defer logger.lock.Unlock()
	logger.formatter = f
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
			level: INFO,
		}
		p = r[pkg]
	}
	return
}

type BaseLogEntry string

func (b BaseLogEntry) LogString() string {
	return string(b)
}
