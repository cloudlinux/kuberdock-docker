// +build !windows

package dockerhooks

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	"github.com/opencontainers/runc/libcontainer/configs"
)

const hookDirPath = "/usr/libexec/oci/hooks.d"

// Prestart function will be called after container process is created but
// before it is started
func Prestart(state configs.HookState, configPath string) error {
	hooks, err := getHooks()
	if err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	for _, item := range hooks {
		if item.Mode().IsRegular() {
			if err := runHook(path.Join(hookDirPath, item.Name()), "prestart", configPath, b); err != nil {
				return err
			}
		}
	}
	return nil
}

// Poststop function will be called after container process has stopped but
// before it is removed
func Poststop(state configs.HookState, configPath string) error {
	hooks, err := getHooks()
	if err != nil {
		return err
	}
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	for i := len(hooks) - 1; i >= 0; i-- {
		fn := hooks[i].Name()
		for _, item := range hooks {
			if item.Mode().IsRegular() && fn == item.Name() {
				if err := runHook(path.Join(hookDirPath, item.Name()), "poststop", configPath, b); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// TotalHooks returns the number of hooks to be used
func TotalHooks() int {
	hooks, _ := getHooks()
	return len(hooks)
}

func getHooks() ([]os.FileInfo, error) {
	// find any hooks executables
	if _, err := os.Stat(hookDirPath); os.IsNotExist(err) {
		return nil, nil
	}
	hooks, err := ioutil.ReadDir(hookDirPath)
	return hooks, err
}

func runHook(hookFilePath string, hookType string, configPath string, stdinBytes []byte) error {
	cmd := exec.Cmd{
		Path: hookFilePath,
		Args: []string{hookFilePath, hookType, configPath},
		Env: []string{
			"container=docker",
		},
		Stdin: bytes.NewReader(stdinBytes),
	}
	return cmd.Run()
}
