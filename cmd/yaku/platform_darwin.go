//go:build darwin

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// platformSetup applies macOS-specific window options.
// Stage 2 Step 1 will refine these once the full macOS implementation is tested.
func platformSetup(opts *options.App) {
	opts.Mac = &mac.Options{
		TitleBar:             mac.TitleBarHiddenInset(),
		Appearance:           mac.NSAppearanceNameDarkAqua,
		WebviewIsTransparent: true,
		WindowIsTranslucent:  true,
		About: &mac.AboutInfo{
			Title:   "Yaku",
			Message: "Real-time audio translation overlay",
		},
	}
}
