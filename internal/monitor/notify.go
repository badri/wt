package monitor

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Notify sends a desktop notification with sound
func Notify(title, message string) error {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = notifyMacOS(title, message)
	case "linux":
		err = notifyLinux(title, message)
	}

	// Ring terminal bell as universal sound fallback
	ringBell()

	return err
}

func notifyMacOS(title, message string) error {
	// Escape double quotes and backslashes for AppleScript
	title = escapeAppleScript(title)
	message = escapeAppleScript(message)

	script := `display notification "` + message + `" with title "` + title + `" sound name "default"`
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func notifyLinux(title, message string) error {
	// Use --urgency=critical to increase chance of sound on some desktop environments
	cmd := exec.Command("notify-send", "--urgency=critical", title, message)
	return cmd.Run()
}

// escapeAppleScript escapes special characters for AppleScript string literals
func escapeAppleScript(s string) string {
	// Escape backslashes first, then double quotes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// ringBell sends a terminal bell character (BEL) for audio feedback
func ringBell() {
	fmt.Print("\a")
}
