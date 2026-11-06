package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// extendPath garante kind e kubectl quando o app é aberto pelo menu do desktop, sem o PATH do shell.
func extendPath() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	extra := []string{filepath.Join(home, ".local/share/mise/shims"), filepath.Join(home, ".local/bin"), "/usr/local/bin"}
	_ = os.Setenv("PATH", os.Getenv("PATH")+":"+strings.Join(extra, ":"))
}

func main() {
	extendPath()
	if handled, code := runCLI(os.Args[1:]); handled {
		os.Exit(code)
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "Faxina",
		Width:     1180,
		Height:    840,
		MinWidth:  820,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 232, G: 238, B: 242, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
