package monitor

import (
	"os/exec"
	"runtime"
)

// Notify sends a desktop notification
func Notify(title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		return notifyMacOS(title, message)
	case "linux":
		return notifyLinux(title, message)
	default:
		// Unsupported platform, silently ignore
		return nil
	}
}

func notifyMacOS(title, message string) error {
	script := `display notification "` + message + `" with title "` + title + `" sound name "default"`
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func notifyLinux(title, message string) error {
	cmd := exec.Command("notify-send", title, message)
	return cmd.Run()
}
