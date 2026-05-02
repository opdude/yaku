//go:build linux

package main

import (
	"os"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// platformSetup applies Linux-specific window options and environment before
// wails.Run is called.
func platformSetup(opts *options.App) {
	// Force XWayland so gtk_window_set_decorated(FALSE) is respected by KDE/GNOME
	// Wayland compositors, which otherwise add server-side decorations regardless.
	os.Setenv("GDK_BACKEND", "x11") //nolint:errcheck

	opts.Linux = &linux.Options{
		Icon:                appIcon,
		ProgramName:         "yaku",
		WebviewGpuPolicy:    linux.WebviewGpuPolicyNever,
		WindowIsTranslucent: false,
	}
}
