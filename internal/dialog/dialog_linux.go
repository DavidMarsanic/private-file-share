//go:build linux

package dialog

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ChooseFiles tries zenity (GNOME/most distros' default), then kdialog
// (KDE) — there's no single file-picker binary guaranteed present on
// every Linux desktop, unlike macOS/Windows.
func ChooseFiles() ([]string, error) {
	if out, err := runPicker("zenity", "--file-selection", "--multiple", "--separator=\n", "--title=Choose files to share"); err == nil {
		if out == "" {
			return nil, nil
		}
		return strings.Split(out, "\n"), nil
	}
	if out, err := runPicker("kdialog", "--getopenfilename", "--multiple", "--separate-output"); err == nil {
		if out == "" {
			return nil, nil
		}
		return strings.Split(out, "\n"), nil
	}
	return nil, fmt.Errorf("no file picker found — install zenity or kdialog")
}

// ChooseFolder tries zenity (GNOME/most distros' default), then kdialog
// (KDE) — there's no single folder-picker binary guaranteed present on
// every Linux desktop, unlike macOS/Windows.
func ChooseFolder() (string, error) {
	if path, err := runPicker("zenity", "--file-selection", "--directory", "--title=Choose a folder to share"); err == nil {
		return path, nil
	}
	if path, err := runPicker("kdialog", "--getexistingdirectory"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("no folder picker found — install zenity or kdialog")
}

func runPicker(name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Nonzero exit here almost always just means the user hit Cancel —
		// treat it as "nothing chosen", not a reason to fail outright.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}
