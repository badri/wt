package main

import (
	"github.com/badri/wt/internal/session"
)

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func collectUsedOffsets(state *session.State) []int {
	var offsets []int
	for _, sess := range state.Sessions {
		if sess.PortOffset > 0 {
			offsets = append(offsets, sess.PortOffset)
		}
	}
	return offsets
}
