package logging

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Logger struct {
	debug   bool
	mu      sync.Mutex
	out     *os.File
	useANSI bool
}

func New(debug bool) *Logger {
	return &Logger{
		debug:   debug,
		out:     os.Stdout,
		useANSI: strings.TrimSpace(os.Getenv("NO_COLOR")) == "",
	}
}

func (l *Logger) log(level string, module string, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		level = "INFO"
	}
	module = strings.ToUpper(strings.TrimSpace(module))
	if module == "" {
		module = "APP"
	}

	timestamp := time.Now().Format("15:04:05.000")
	message := fmt.Sprintf(format, args...)

	if l.useANSI {
		fmt.Fprintf(
			l.out,
			"%s[%s]%s%s[%s]%s%s[%s]%s %s\n",
			colorForTimestamp(),
			timestamp,
			colorReset(),
			colorForLevel(level),
			level,
			colorReset(),
			colorForModule(module),
			module,
			colorReset(),
			message,
		)
		return
	}

	fmt.Fprintf(l.out, "[%s][%s][%s] %s\n", timestamp, level, module, message)
}

func (l *Logger) IsDebug() bool {
	return l.debug
}

func (l *Logger) Mask(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 8 {
		return strings.Repeat("*", len(trimmed))
	}
	return trimmed[:4] + strings.Repeat("*", len(trimmed)-8) + trimmed[len(trimmed)-4:]
}

func (l *Logger) KV(key string, value any) string {
	return fmt.Sprintf("%s=%v", key, value)
}

func (l *Logger) Info(format string, args ...any) {
	l.log("info", "app", format, args...)
}

func (l *Logger) Warn(format string, args ...any) {
	l.log("warn", "app", format, args...)
}

func (l *Logger) Error(format string, args ...any) {
	l.log("error", "app", format, args...)
}

func (l *Logger) Debug(format string, args ...any) {
	if !l.debug {
		return
	}
	l.log("debug", "app", format, args...)
}

func (l *Logger) InfoModule(module string, format string, args ...any) {
	l.log("info", module, format, args...)
}

func (l *Logger) WarnModule(module string, format string, args ...any) {
	l.log("warn", module, format, args...)
}

func (l *Logger) ErrorModule(module string, format string, args ...any) {
	l.log("error", module, format, args...)
}

func (l *Logger) DebugModule(module string, format string, args ...any) {
	if !l.debug {
		return
	}
	l.log("debug", module, format, args...)
}

func colorForTimestamp() string {
	return "\x1b[38;5;244m"
}

func colorForLevel(level string) string {
	switch level {
	case "DEBUG":
		return "\x1b[36m"
	case "INFO":
		return "\x1b[32m"
	case "WARN":
		return "\x1b[33m"
	case "ERROR":
		return "\x1b[31m"
	default:
		return "\x1b[37m"
	}
}

func colorForModule(module string) string {
	switch module {
	case "HTTP":
		return "\x1b[38;5;39m"
	case "AUTH":
		return "\x1b[38;5;141m"
	case "UPSTREAM":
		return "\x1b[38;5;81m"
	case "OPENAI":
		return "\x1b[38;5;117m"
	case "ACCOUNT":
		return "\x1b[38;5;214m"
	case "APP":
		return "\x1b[38;5;252m"
	default:
		return "\x1b[38;5;250m"
	}
}

func colorReset() string {
	return "\x1b[0m"
}
