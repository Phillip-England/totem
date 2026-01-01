package main

import (
	"bufio"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"

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
	InitDB()

	app := vii.NewApp()
	app.Use(vii.MwLogger)

	// Load templates from the "templates" directory
	err := app.Templates("templates", template.FuncMap{})
	if err != nil {
		panic(err)
	}

	app.At("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			Title    string
			Message  string
			ShowForm bool
		}{
			Title:    "Login",
			Message:  "Please Login",
			ShowForm: true,
		}
		err := vii.ExecuteTemplate(w, r, "index.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	app.At("POST /{$}", func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		password := r.FormValue("password")

		adminUser := os.Getenv("ADMIN_USERNAME")
		adminPass := os.Getenv("ADMIN_PASSWORD")

		if username == adminUser && password == adminPass {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}

		data := struct {
			Title    string
			Message  string
			ShowForm bool
		}{
			Title:    "Login Status",
			Message:  "Login Failed. Try again.",
			ShowForm: true,
		}

		err := vii.ExecuteTemplate(w, r, "index.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Admin Dashboard - List Locations
	app.At("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		locations, err := GetAllLocations()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			Locations []CfaLocation
		}{
			Locations: locations,
		}

		err = vii.ExecuteTemplate(w, r, "admin.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Create Location
	app.At("POST /admin/locations", func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		number := r.FormValue("number")
		if name != "" && number != "" {
			err := CreateLocation(name, number)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	})

	// Delete Location
	app.At("POST /admin/locations/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		err = DeleteLocation(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	})

	// Edit Location Form
	app.At("GET /admin/locations/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}
		data := struct {
			Location CfaLocation
		}{
			Location: loc,
		}
		err = vii.ExecuteTemplate(w, r, "edit_location.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Update Location
	app.At("POST /admin/locations/{id}/update", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		name := r.FormValue("name")
		number := r.FormValue("number")
		err = UpdateLocation(id, name, number)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	})

	err = app.Serve("8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}