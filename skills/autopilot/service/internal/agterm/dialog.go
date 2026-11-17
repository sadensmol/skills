package agterm

import "strings"

// Dialog is a Claude Code start dialog that the service answers.
type Dialog string

const (
	DialogBypass Dialog = "bypass"
	DialogTrust  Dialog = "trust"
)

const (
	bypassDialogHead = "Bypass Permissions mode"
	bypassDialogYes  = "Yes, I accept"
	trustDialogHead  = "Quick safety check"
	trustDialogYes   = "Yes, I trust this folder"
	// readyMarker is the footer of the Claude Code prompt in bypass mode: no start dialog is open any more.
	readyMarker = "bypass permissions on"
	keyDown     = "\x1b[B"
	keyEnter    = "\r"
)

// DialogAnswer returns the dialog on the screen and the bytes that accept it, and false for any other text.
// Only the exact text of a known dialog is answered, so a session that waits for anything else stays blocked for a person.
// Both dialogs show "No" first and "Yes" second.
func DialogAnswer(screen string) (Dialog, []string, bool) {
	switch {
	case strings.Contains(screen, bypassDialogHead) && strings.Contains(screen, bypassDialogYes):
		return DialogBypass, []string{keyDown, keyEnter}, true
	case strings.Contains(screen, trustDialogHead) && strings.Contains(screen, trustDialogYes):
		return DialogTrust, []string{keyDown, keyEnter}, true
	}
	return "", nil, false
}

// Ready reports whether the screen shows the Claude Code prompt in bypass mode, past every start dialog.
func Ready(screen string) bool {
	return strings.Contains(screen, readyMarker)
}
