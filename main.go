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
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "ORCH",
		Width:            1440,
		Height:           900,
		MinWidth:         960,
		MinHeight:        600,
		DisableResize:    false,
		Fullscreen:       false,
		Frameless:        false,
		StartHidden:      false,
		BackgroundColour: &options.RGBA{R: 15, G: 15, B: 20, A: 255}, // #0f0f14 (Dark theme bg)
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnDomReady: app.DomReady,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               false,
				FullSizeContent:            true,
				UseToolbar:                 false,
			},
			About: &mac.AboutInfo{
				Title:   "ORCH",
				Message: "Orchestrator Colaborativo de IA & GitHub",
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
			OnUrlOpen:            app.HandleDeepLink,
		},
	})

	if err != nil {
		log.Fatalf("[ORCH] Fatal: %v", err)
	}
}
