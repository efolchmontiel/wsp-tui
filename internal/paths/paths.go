package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Paths holds OS-appropriate locations for wsp-tui data.
type Paths struct {
	DataDir    string
	StateDir   string
	ConfigDir  string
	SessionDB  string
	AppDB      string
	MediaDir   string
	CacheDir   string
	LogFile    string
	ConfigFile string
}

// Resolve returns absolute paths, creating directories as needed.
//
// Linux / Arch / macOS: XDG (~/.local/share, ~/.local/state, ~/.config)
// Windows: %LOCALAPPDATA%\whatstui and %APPDATA%\whatstui
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("home dir: %w", err)
	}

	var dataHome, stateHome, configHome string
	switch runtime.GOOS {
	case "windows":
		local := envOr("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
		roaming := envOr("APPDATA", filepath.Join(home, "AppData", "Roaming"))
		dataHome = local
		stateHome = local
		configHome = roaming
	default:
		dataHome = envOr("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
		stateHome = envOr("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
		configHome = envOr("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	}

	p := Paths{
		DataDir:    filepath.Join(dataHome, "whatstui"),
		StateDir:   filepath.Join(stateHome, "whatstui"),
		ConfigDir:  filepath.Join(configHome, "whatstui"),
		SessionDB:  filepath.Join(dataHome, "whatstui", "session.db"),
		AppDB:      filepath.Join(dataHome, "whatstui", "whatstui.db"),
		MediaDir:   filepath.Join(dataHome, "whatstui", "media"),
		CacheDir:   filepath.Join(dataHome, "whatstui", "cache"),
		LogFile:    filepath.Join(stateHome, "whatstui", "whatstui.log"),
		ConfigFile: filepath.Join(configHome, "whatstui", "config.toml"),
	}

	dirs := []string{
		p.DataDir,
		p.StateDir,
		p.ConfigDir,
		p.CacheDir,
		filepath.Join(p.MediaDir, "images"),
		filepath.Join(p.MediaDir, "videos"),
		filepath.Join(p.MediaDir, "audio"),
		filepath.Join(p.MediaDir, "documents"),
		filepath.Join(p.MediaDir, "stickers"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Paths{}, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	return p, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
