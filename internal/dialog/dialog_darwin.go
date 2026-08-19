//go:build darwin

// Package dialog opens the OS's native file/folder-choose dialogs. A
// browser <input type=file> can pick files but never hands back a real
// filesystem path (sandboxed by design) — this app needs actual paths to
// serve straight off disk, so the picker has to come from the OS, not
// the page.
package dialog

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ChooseFiles opens the native Finder file-choose dialog (multi-select)
// and returns the chosen absolute paths, or nil if the user canceled.
func ChooseFiles() ([]string, error) {
	// Joined with a literal newline from inside AppleScript itself, not
	// its default ", " list-to-string coercion — a path could plausibly
	// contain ", " but never an actual newline.
	script := `tell application "Finder" to activate
set theFiles to choose file with prompt "Choose files to share" with multiple selections allowed
set thePaths to (POSIX path of item 1 of theFiles)
repeat with i from 2 to (count of theFiles)
	set thePaths to thePaths & linefeed & (POSIX path of item i of theFiles)
end repeat
return thePaths`
	out, err := runAppleScript(script)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ChooseFolder opens the native Finder folder-choose dialog and returns
// the chosen absolute path, or "" if the user canceled.
func ChooseFolder() (string, error) {
	return runAppleScript(`tell application "Finder" to activate
POSIX path of (choose folder with prompt "Choose a folder to share")`)
}

// runAppleScript activates Finder first — without it, the dialog is
// owned by a background process (this binary has no Dock icon/foreground
// app status of its own), so macOS can open it behind the Chrome
// app-mode window instead of in front, which looks exactly like the
// button did nothing.
func runAppleScript(script string) (string, error) {
	cmd := exec.Command("osascript", "-e", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "User canceled") {
			return "", nil
		}
		return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
