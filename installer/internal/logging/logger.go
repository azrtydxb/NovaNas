// Package logging provides a simple file+stderr logger for the installer.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// DefaultLogPath is where we write the persistent install log inside the
// running live environment.
const DefaultLogPath = "/var/log/novanas-installer.log"

// Logger wraps log.Logger plus an optional stderr mirror.
type Logger struct {
	mu    sync.Mutex
	file  *os.File
	debug bool
	out   *log.Logger
}

// New opens (creates, appends) the log file. If the file cannot be opened
// (e.g. running as a non-root dev), it falls back to stderr-only logging.
func New(path string, debug bool) *Logger {
	var sinks []io.Writer
	sinks = append(sinks, os.Stderr)

	var f *os.File
	if path != "" {
		ff, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			f = ff
			sinks = append(sinks, ff)
		}
	}
	mw := io.MultiWriter(sinks...)
	return &Logger{
		file:  f,
		debug: debug,
		out:   log.New(mw, "", 0),
	}
}

// Close flushes and closes the log file (if any).
func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *Logger) emit(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out.Printf("%s [%s] %s", time.Now().UTC().Format(time.RFC3339), level, msg)
}

// Infof logs an informational line.
func (l *Logger) Infof(format string, args ...any) {
	l.emit("INFO", fmt.Sprintf(format, args...))
}

// Warnf logs a warning.
func (l *Logger) Warnf(format string, args ...any) {
	l.emit("WARN", fmt.Sprintf(format, args...))
}

// Errorf logs an error.
func (l *Logger) Errorf(format string, args ...any) {
	l.emit("ERROR", fmt.Sprintf(format, args...))
}

// Debugf logs only when debug mode is enabled.
func (l *Logger) Debugf(format string, args ...any) {
	if !l.debug {
		return
	}
	l.emit("DEBUG", fmt.Sprintf(format, args...))
}
