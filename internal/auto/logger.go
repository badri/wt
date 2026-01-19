package auto

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger handles logging for auto runs
type Logger struct {
	file      *os.File
	startTime time.Time
}

// NewLogger creates a new logger for this auto run
func NewLogger(logsDir string) (*Logger, error) {
	startTime := time.Now()
	filename := fmt.Sprintf("auto-%s.log", startTime.Format("2006-01-02-150405"))
	path := filepath.Join(logsDir, filename)

	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	logger := &Logger{
		file:      file,
		startTime: startTime,
	}

	logger.Log("=== wt auto started at %s ===", startTime.Format(time.RFC3339))

	return logger, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	l.Log("=== wt auto finished at %s (duration: %v) ===", time.Now().Format(time.RFC3339), time.Since(l.startTime))
	return l.file.Close()
}

// Log writes a log entry
func (l *Logger) Log(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s\n", timestamp, msg)
	l.file.WriteString(line)
}

// LogBeadStart logs the start of bead processing
func (l *Logger) LogBeadStart(beadID, title string) {
	l.Log("BEAD_START: %s - %s", beadID, title)
}

// LogBeadEnd logs the end of bead processing
func (l *Logger) LogBeadEnd(beadID, outcome string, duration time.Duration) {
	l.Log("BEAD_END: %s - outcome=%s duration=%v", beadID, outcome, duration)
}
