//go:build windows

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// platformSetup applies Windows-specific window options.
// Stage 2 Step 1 will refine these once the full Windows implementation is tested.
func platformSetup(opts *options.App) {
	opts.Windows = &windows.Options{
		WebviewIsTransparent:              true,
		WindowIsTranslucent:               true,
		DisableWindowIcon:                 true,
		IsZoomControlEnabled:              false,
		DisableFramelessWindowDecorations: false,
	}
}
