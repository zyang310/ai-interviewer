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

func main() {
	app, err := NewApp()
	if err != nil {
		log.Fatalf("Failed to initialise application: %v", err)
	}

	err = wails.Run(&options.App{
		Title:     "AI Interviewer",
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
