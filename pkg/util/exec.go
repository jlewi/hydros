package util

import (
	"os/exec"

	"github.com/go-logr/logr"
)

// ExecHelper is a wrapper for executing shell commands.
type ExecHelper struct {
	Log logr.Logger
}

// CmdSetter is an interface for setting commands.
type CmdSetter func(cmd *exec.Cmd)

// RunCommands runs multiple commands.
// CmdSetter is a function to configure the command.
func (h *ExecHelper) RunCommands(commands [][]string, setter CmdSetter) error {
	for _, c := range commands {
		cmd := exec.Command(c[0], c[1:]...)

		if setter != nil {
			setter(cmd)
		}

		if err := h.Run(cmd); err != nil {
			return err
		}
	}
	return nil
}

// Run runs and logs stdout/stderr
func (h *ExecHelper) Run(cmd *exec.Cmd) error {
	log := h.Log
	data, err := h.RunQuietly(cmd)
	if err != nil {
		log.Error(err, "Shell command failed", "command", cmd.String(), "dir", cmd.Dir, "output", data)
		return err
	}

	log.V(Debug).Info("Shell Command succeeded", "command", cmd.String(), "dir", cmd.Dir, "output", data)

	return nil
}

// RunQuietly runs without logging stdout/stderr. Use this method when
// you want to let the caller decide whether to log or not. A common
// use case would be when commands failing to run doesn't necessarily
// indicate an error.
func (h *ExecHelper) RunQuietly(cmd *exec.Cmd) (string, error) {
	data, err := cmd.CombinedOutput()
	return string(data), err
}
