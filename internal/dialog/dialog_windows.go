//go:build windows

package dialog

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ChooseFiles opens the native file-open dialog (multi-select) via
// PowerShell's System.Windows.Forms.OpenFileDialog and returns the
// chosen paths, or nil if the user canceled.
func ChooseFiles() ([]string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.OpenFileDialog
$dialog.Title = "Choose files to share"
$dialog.Multiselect = $true
if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
    $dialog.FileNames | ForEach-Object { Write-Output $_ }
}`
	out, err := runPowerShell(script)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n"), nil
}

// ChooseFolder opens the native folder-browser dialog via PowerShell's
// System.Windows.Forms.FolderBrowserDialog and returns the chosen path,
// or "" if the user canceled.
func ChooseFolder() (string, error) {
	script := `Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = "Choose a folder to share"
if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
    Write-Output $dialog.SelectedPath
}`
	return runPowerShell(script)
}

func runPowerShell(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
