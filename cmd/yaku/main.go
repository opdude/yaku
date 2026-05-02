package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app := &App{}

	opts := &options.App{
		Title:     "Yaku",
		Width:     900,
		Height:    260, // source panel hidden by default; JS expands when shown
		MinWidth:  400,
		MinHeight: 160,

		// Frameless removes the OS title bar; dragging uses --wails-draggable CSS.
		Frameless: true,

		// Request a transparent window frame from the compositor.
		// The CSS body paints a semi-transparent dark layer on top.
		BackgroundColour: &options.RGBA{R: 8, G: 8, B: 14, A: 255},

		AssetServer: &assetserver.Options{Assets: assets},

		OnStartup:  app.startup,
		OnShutdown: app.shutdown,

		Bind: []any{app},
	}

	// Apply platform-specific window options (Linux GTK, Windows WebView2, macOS WKWebView).
	platformSetup(opts)

	if err := wails.Run(opts); err != nil {
		panic(err)
	}
}
