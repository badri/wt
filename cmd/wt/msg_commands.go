package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/msg"
)

func cmdMsg(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return cmdMsgHelp()
	}

	switch args[0] {
	case "send":
		return cmdMsgSend(cfg, args[1:])
	case "recv":
		return cmdMsgRecv(cfg, args[1:])
	case "list":
		return cmdMsgList(cfg, args[1:])
	default:
		return fmt.Errorf("unknown msg subcommand: %s", args[0])
	}
}

func cmdMsgSend(cfg *config.Config, args []string) error {
	var to, subject, body, thread string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--to":
			if i+1 < len(args) {
				to = args[i+1]
				i++
			}
		case "--subject", "-s":
			if i+1 < len(args) {
				subject = args[i+1]
				i++
			}
		case "--body", "-b":
			if i+1 < len(args) {
				body = args[i+1]
				i++
			}
		case "--thread", "-t":
			if i+1 < len(args) {
				thread = args[i+1]
				i++
			}
		}
	}

	if to == "" || subject == "" {
		return fmt.Errorf("usage: wt msg send --to <recipient> --subject <TASK|DONE|STUCK|PROGRESS> --body <text> [--thread <id>]")
	}

	subj := msg.Subject(strings.ToUpper(subject))
	switch subj {
	case msg.SubjectTask, msg.SubjectDone, msg.SubjectStuck, msg.SubjectProgress:
	default:
		return fmt.Errorf("invalid subject %q: must be TASK, DONE, STUCK, or PROGRESS", subject)
	}

	store, err := msg.Open(msgDBPath(cfg))
	if err != nil {
		return err
	}
	defer store.Close()

	id, err := store.Send(&msg.Message{
		Subject:  subj,
		From:     "cli",
		To:       to,
		Body:     body,
		ThreadID: thread,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Sent message #%d [%s] to %s\n", id, subj, to)
	return nil
}

func cmdMsgRecv(cfg *config.Config, args []string) error {
	var as string
	for i := 0; i < len(args); i++ {
		if args[i] == "--as" && i+1 < len(args) {
			as = args[i+1]
			i++
		}
	}
	if as == "" {
		return fmt.Errorf("usage: wt msg recv --as <identity>")
	}

	store, err := msg.Open(msgDBPath(cfg))
	if err != nil {
		return err
	}
	defer store.Close()

	msgs, err := store.Recv(as)
	if err != nil {
		return err
	}

	if len(msgs) == 0 {
		fmt.Println("No unacked messages.")
		return nil
	}

	for _, m := range msgs {
		fmt.Printf("#%d [%s] from=%s thread=%s\n", m.ID, m.Subject, m.From, m.ThreadID)
		if m.Body != "" {
			fmt.Printf("  %s\n", m.Body)
		}
		if err := store.Ack(m.ID); err != nil {
			fmt.Printf("  Warning: failed to ack: %v\n", err)
		}
	}
	fmt.Printf("\n%d message(s) received and acked.\n", len(msgs))
	return nil
}

func cmdMsgList(cfg *config.Config, args []string) error {
	var thread string
	for i := 0; i < len(args); i++ {
		if (args[i] == "--thread" || args[i] == "-t") && i+1 < len(args) {
			thread = args[i+1]
			i++
		}
	}

	store, err := msg.Open(msgDBPath(cfg))
	if err != nil {
		return err
	}
	defer store.Close()

	msgs, err := store.List(thread)
	if err != nil {
		return err
	}

	if len(msgs) == 0 {
		fmt.Println("No messages.")
		return nil
	}

	for _, m := range msgs {
		acked := " "
		if m.AckedAt != nil {
			acked = "✓"
		}
		fmt.Printf("[%s] #%d [%s] %s → %s  thread=%s\n", acked, m.ID, m.Subject, m.From, m.To, m.ThreadID)
		if m.Body != "" {
			fmt.Printf("       %s\n", m.Body)
		}
	}
	fmt.Printf("\n%d message(s) total.\n", len(msgs))
	return nil
}

func cmdMsgHelp() error {
	fmt.Println(`wt msg - Message store for worker coordination

Subcommands:
  send    Send a message
  recv    Receive and ack unacked messages
  list    List all messages

Usage:
  wt msg send --to <recipient> --subject <TASK|DONE|STUCK|PROGRESS> --body <text> [--thread <id>]
  wt msg recv --as <identity>
  wt msg list [--thread <id>]`)
	return nil
}

func msgDBPath(cfg *config.Config) string {
	return filepath.Join(cfg.ConfigDir(), "messages.db")
}
