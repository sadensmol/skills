package cli

import (
	"errors"
	"fmt"
	"github.com/sadensmol/autopilot/internal/controller/socket"
	"os"
	"path/filepath"
)

const socketEnv = "AUTOPILOT_SOCKET"

// settingsRel is the folder that marks the workspace folder; the socket lives in it.
var settingsRel = filepath.Join(".workspace", "autopilot")

// SocketPath returns AUTOPILOT_SOCKET, else the socket of the nearest folder above cwd that holds .workspace/autopilot/.
func SocketPath(env Env) (string, error) {
	if p := env.Getenv(socketEnv); p != "" {
		return p, nil
	}
	cwd, err := env.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot read the working folder: %w", err)
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(filepath.Join(dir, settingsRel))
		if err == nil && info.IsDir() {
			return filepath.Join(dir, settingsRel, socket.FileName), nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, os.ErrPermission) {
			return "", fmt.Errorf("cannot check %s: %w", dir, err)
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no folder with %s above %s: set %s or run inside the workspace folder", filepath.ToSlash(settingsRel)+"/", cwd, socketEnv)
		}
	}
}
