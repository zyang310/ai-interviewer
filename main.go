package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is the app's release version, injected at build time via
// -ldflags "-X main.version=vX.Y.Z" (see .github/workflows/release.yml). It
// stays "dev" for local builds, which suppresses the in-app update check.
var version = "dev"

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatalf("Failed to initialise application: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     "Mogi",
		Width:     1024,
		Height:    768,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// Transparent window so the floating overlay bar hovers over the user's
		// IDE with the surrounding area see-through. Non-overlay screens paint
		// their own opaque background (.app / .setup-root in CSS).
		BackgroundColour: &options.RGBA{R: 17, G: 19, B: 23, A: 0},
		Mac: &mac.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatalf("Wails error: %v", err)
	}
}
