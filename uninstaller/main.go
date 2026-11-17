package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	headless := flag.Bool("headless", false, "Run uninstallation silently without prompts")
	flag.Parse()

	fmt.Println("Nozik Uninstaller starting...")

	if !*headless {
		fmt.Print("Are you sure you want to uninstall Nozik? (y/N): ")
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Uninstallation cancelled.")
			return
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}

	var installDir string
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		installDir = filepath.Join(local, "nozik")
	case "darwin":
		installDir = filepath.Join(home, "Library", "Application Support", "nozik")
	default:
		installDir = filepath.Join(home, ".local", "share", "nozik")
	}

	fmt.Printf("Removing installation directory: %s\n", installDir)
	if err := os.RemoveAll(installDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning removing installation directory: %v\n", err)
	}

	// Platform specific cleanup
	switch runtime.GOOS {
	case "windows":
		// Remove shortcuts and environment path and registry
		desktop := filepath.Join(home, "Desktop")
		startMenu := filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs")
		_ = os.Remove(filepath.Join(desktop, "Nozik.lnk"))
		_ = os.Remove(filepath.Join(startMenu, "Nozik.lnk"))

		// We can try calling reg delete command
		cmd := exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\Nozik`, "/f")
		_ = cmd.Run()

	case "darwin":
		appDst := filepath.Join(home, "Applications", "Nozik.app")
		fmt.Printf("Removing App bundle: %s\n", appDst)
		if err := os.RemoveAll(appDst); err != nil {
			fmt.Fprintf(os.Stderr, "Warning removing App bundle: %v\n", err)
		}

	default: // Linux / FreeBSDs
		binLink := filepath.Join(home, ".local", "bin", "nozik")
		fmt.Printf("Removing symlink: %s\n", binLink)
		_ = os.Remove(binLink)

		desktopFile := filepath.Join(home, ".local", "share", "applications", "nozik.desktop")
		fmt.Printf("Removing desktop entry: %s\n", desktopFile)
		_ = os.Remove(desktopFile)

		iconsDir := filepath.Join(home, ".local", "share", "icons", "hicolor")
		fmt.Println("Removing icons...")
		// Remove every nozik icon file under the hicolor tree, regardless of
		// the sizes that were actually installed.
		_ = filepath.Walk(iconsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && filepath.Base(path) == "nozik.png" {
				_ = os.Remove(path)
			}
			return nil
		})

		// Update desktop database and icon cache
		_ = exec.Command("update-desktop-database", "-q", filepath.Join(home, ".local", "share", "applications")).Run()
		_ = exec.Command("gtk-update-icon-cache", "-f", "-t", iconsDir).Run()
	}

	fmt.Println("Nozik has been successfully uninstalled.")
}
