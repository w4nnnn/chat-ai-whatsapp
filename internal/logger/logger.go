package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	LevelError Level = iota
	LevelWarn
	LevelInfo
	LevelDebug
)

var levelNames = map[Level]string{
	LevelError: "ERROR",
	LevelWarn:  "WARN",
	LevelInfo:  "INFO",
	LevelDebug: "DEBUG",
}

type Logger struct {
	mu     sync.Mutex
	level  Level
	logger *log.Logger
}

var defaultLogger *Logger

func init() {
	level := LevelInfo
	envLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	switch envLevel {
	case "debug":
		level = LevelDebug
	case "info":
		level = LevelInfo
	case "warn":
		level = LevelWarn
	case "error":
		level = LevelError
	}
	defaultLogger = &Logger{
		level:  level,
		logger: log.New(os.Stdout, "", 0),
	}
}

func Default() *Logger { return defaultLogger }

func (l *Logger) log(level Level, format string, args ...any) {
	if level > l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	prefix := fmt.Sprintf("[%s] [%s] ", timestamp, levelNames[level])
	output := prefix + msg + "\n"

	if level == LevelError {
		fmt.Fprint(os.Stderr, output)
	} else {
		fmt.Fprint(os.Stdout, output)
	}
}

func (l *Logger) Error(format string, args ...any) { l.log(LevelError, format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.log(LevelWarn, format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.log(LevelInfo, format, args...) }
func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, format, args...) }

func Error(format string, args ...any) { defaultLogger.Error(format, args...) }
func Warn(format string, args ...any)  { defaultLogger.Warn(format, args...) }
func Info(format string, args ...any)  { defaultLogger.Info(format, args...) }
func Debug(format string, args ...any) { defaultLogger.Debug(format, args...) }
