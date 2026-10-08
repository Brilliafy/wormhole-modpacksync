package main

import (
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"wormhole-sync/pkg/signaling"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	relayFlag := flag.Bool("relay", false, "Run as standalone signaling relay server")
	relayPort := flag.Int("port", 42425, "Port for signaling relay server")
	flag.Parse()

	if *relayFlag {
		server := signaling.NewEmbeddedRelayServer()
		addr := fmt.Sprintf("0.0.0.0:%d", *relayPort)
		fmt.Printf("Wormhole Signaling Relay Server listening on %s\n", addr)
		if err := http.ListenAndServe(addr, server.Handler()); err != nil {
			fmt.Printf("Relay error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Wormhole Modpack Sync",
		Width:  1080,
		Height: 760,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 12, G: 15, B: 23, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
