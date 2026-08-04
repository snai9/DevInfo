package dialog

import (
	"os/exec"
	"strings"
)

var engine string

func init() {
	if _, err := exec.LookPath("kdialog"); err == nil {
		engine = "kdialog"
	} else if _, err := exec.LookPath("zenity"); err == nil {
		engine = "zenity"
	}
}

func Available() bool {
	return engine != ""
}

func Engine() string {
	return engine
}

func Entry(title, text, defaultVal string) (string, bool) {
	switch engine {
	case "kdialog":
		cmd := exec.Command("kdialog",
			"--title", title,
			"--inputbox", text, defaultVal,
		)
		out, err := cmd.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	case "zenity":
		cmd := exec.Command("zenity", "--entry",
			"--title="+title,
			"--text="+text,
			"--entry-text="+defaultVal,
			"--width=500",
		)
		out, err := cmd.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	}
	return "", false
}

func Info(title, text string) {
	switch engine {
	case "kdialog":
		exec.Command("kdialog", "--title", title, "--msgbox", text).Run()
	case "zenity":
		exec.Command("zenity", "--info",
			"--title="+title,
			"--text="+text,
			"--width=450",
		).Run()
	}
}

func Error(title, text string) {
	switch engine {
	case "kdialog":
		exec.Command("kdialog", "--title", title, "--error", text).Run()
	case "zenity":
		exec.Command("zenity", "--error",
			"--title="+title,
			"--text="+text,
			"--width=450",
		).Run()
	}
}

func YesNo(title, text string) bool {
	switch engine {
	case "kdialog":
		cmd := exec.Command("kdialog", "--title", title, "--yesno", text)
		return cmd.Run() == nil
	case "zenity":
		cmd := exec.Command("zenity", "--question",
			"--title="+title,
			"--text="+text,
			"--width=450",
		)
		return cmd.Run() == nil
	}
	return false
}

type Progress struct {
	cmd *exec.Cmd
}

func ShowProgress(title, text string) *Progress {
	var cmd *exec.Cmd
	switch engine {
	case "kdialog":
		cmd = exec.Command("kdialog",
			"--title", title,
			"--progressbar", text, "0",
		)
	case "zenity":
		cmd = exec.Command("zenity", "--progress",
			"--title="+title,
			"--text="+text,
			"--pulsate",
			"--auto-close",
			"--width=400",
		)
	}
	if cmd != nil {
		cmd.Start()
	}
	return &Progress{cmd: cmd}
}

func (p *Progress) Close() {
	if p != nil && p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}
