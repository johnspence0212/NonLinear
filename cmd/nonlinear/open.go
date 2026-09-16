package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func openAppWindow(url string) {
	go func() {
		time.Sleep(250 * time.Millisecond)
		if err := launchApp(url); err != nil {
			fmt.Fprintf(os.Stderr, "nonlinear  window: %v\n", err)
			fmt.Fprintf(os.Stderr, "nonlinear  install the PWA from Chrome, or: chrome --app=%s\n", url)
		}
	}()
}

func launchApp(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-na", "Google Chrome", "--args", "--app="+url).Start()
	}
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "start", "chrome", "--app="+url).Start()
	}
	for _, bin := range []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"microsoft-edge",
		"microsoft-edge-stable",
		"brave-browser",
	} {
		if p, err := exec.LookPath(bin); err == nil {
			return exec.Command(p, "--app="+url, "--new-window").Start()
		}
	}
	return fmt.Errorf("no chrome/chromium/edge found")
}
