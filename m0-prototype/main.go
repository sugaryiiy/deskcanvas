package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	assets, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	app := NewApp()

	err = wails.Run(&options.App{
		Title:         "DeskCanvas M0",
		Width:         1280,
		Height:        800,
		Frameless:     true,
		DisableResize: true,
		// Fully transparent window + webview: only the cards painted by the
		// frontend are visible, everything else must show the wallpaper
		// through. This is what makes the canvas read as "part of the
		// desktop" instead of a window.
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: app, // /api/* state + mode endpoints (see app.go)
		},
		Mac: &mac.Options{
			// Note: Wails v2.16 has no ActivationPolicy option (it is
			// commented out in the framework), so the no-Dock-icon /
			// no-focus-stealing behaviour is applied natively in
			// platform_darwin.m instead.
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			TitleBar:             mac.TitleBarHidden(),
		},
		OnStartup:  app.startup,
		OnDomReady: app.domReady,
	})
	if err != nil {
		log.Fatal(err)
	}
}
