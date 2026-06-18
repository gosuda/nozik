package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gg582/nozik/internal/config"
	"github.com/gg582/nozik/internal/db"
	"github.com/gg582/nozik/internal/mcp"
	"github.com/gg582/nozik/internal/ui"
)

func main() {
	if hasHelpFlag(os.Args[1:]) {
		printHelp()
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runMCP()
		return
	}

	if err := config.EnsureAppDir(); err != nil {
		log.Fatal(err)
	}
	database, err := db.Open(config.DBPath())
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer database.Close()

	app, err := ui.NewApp(database)
	if err != nil {
		log.Fatal("create app:", err)
	}
	code := app.Run()
	os.Exit(code)
}

func runMCP() {
	if err := config.EnsureAppDir(); err != nil {
		log.Fatal(err)
	}
	database, err := db.Open(config.DBPath())
	if err != nil {
		log.Fatal("open db:", err)
	}
	defer database.Close()
	server := mcp.NewServer(database)
	if err := server.Run(); err != nil {
		log.Fatal("mcp server:", err)
	}
}

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func printHelp() {
	fmt.Print(`Nozik - AI music agent for Gozik

Usage:
  nozik            Launch the GTK dashboard
  nozik mcp        Run the stdio MCP server
  nozik --help     Show this help

Nozik calls Gozik over gRPC (default 127.0.0.1:50051).
Set GOZIK_GRPC_ADDR to override the target address.

AI providers (paste API key in Settings, or set env fallback):
  Gemini   Settings / GEMINI_API_KEY
  ChatGPT  Settings / OPENAI_API_KEY
  Grok     Settings / GROK_API_KEY
  Kimi     Settings / KIMI_API_KEY
  Others   Custom Luau script via Settings dashboard

Taste database is stored at:
  ` + config.DBPath() + `
`)
}
