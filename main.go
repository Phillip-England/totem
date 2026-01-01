package main

import (
	"bufio"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/phillip-england/totem/pkg/data"
	"github.com/phillip-england/totem/pkg/handlers"
	"github.com/phillip-england/vii"
)

func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {
		fmt.Println("Error loading .env:", err)
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
}

func main() {
	loadEnv()
	data.InitDB()

	app := vii.NewApp()
	app.Use(vii.MwLogger)

	// Load templates from the "templates" directory
	err := app.Templates("templates", template.FuncMap{})
	if err != nil {
		panic(err)
	}

	handlers.RegisterRoutes(&app)

	err = app.Serve("8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}
