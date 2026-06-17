package config

import (
	"os"
	"path/filepath"
)

// AppDir returns the directory used for Nozik configuration/data.
func AppDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "nozik")
}

// DBPath returns the default SQLite database path.
func DBPath() string {
	return filepath.Join(AppDir(), "taste.db")
}

// SettingsPath returns the persisted settings JSON path.
func SettingsPath() string {
	return filepath.Join(AppDir(), "settings.json")
}

// EnsureAppDir creates the Nozik config directory.
func EnsureAppDir() error {
	return os.MkdirAll(AppDir(), 0755)
}

// GozikAddr returns the gRPC address used by Gozik.
func GozikAddr() string {
	if addr := os.Getenv("GOZIK_GRPC_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:50051"
}

// EnvKey returns an API key from the environment.
func EnvKey(name string) string {
	return os.Getenv(name)
}
