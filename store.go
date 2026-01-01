package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

type CfaLocation struct {
	ID     int
	Name   string
	Number string
}

func InitDB() {
	if _, err := os.Stat("data"); os.IsNotExist(err) {
		os.Mkdir("data", 0755)
	}

	dbPath := filepath.Join("data", "totem.db")
	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		panic(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS locations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		number TEXT
	);`

	_, err = DB.Exec(createTableSQL)
	if err != nil {
		panic(err)
	}
	fmt.Println("Database initialized successfully.")
}

// Create
func CreateLocation(name, number string) error {
	stmt, err := DB.Prepare("INSERT INTO locations(name, number) VALUES(?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(name, number)
	return err
}

// Read All
func GetAllLocations() ([]CfaLocation, error) {
	rows, err := DB.Query("SELECT id, name, number FROM locations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locations []CfaLocation
	for rows.Next() {
		var loc CfaLocation
		err = rows.Scan(&loc.ID, &loc.Name, &loc.Number)
		if err != nil {
			return nil, err
		}
		locations = append(locations, loc)
	}
	return locations, nil
}

// Read One
func GetLocationByID(id int) (CfaLocation, error) {
	var loc CfaLocation
	row := DB.QueryRow("SELECT id, name, number FROM locations WHERE id = ?", id)
	err := row.Scan(&loc.ID, &loc.Name, &loc.Number)
	return loc, err
}

// Update
func UpdateLocation(id int, name, number string) error {
	stmt, err := DB.Prepare("UPDATE locations SET name = ?, number = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(name, number, id)
	return err
}

// Delete
func DeleteLocation(id int) error {
	stmt, err := DB.Prepare("DELETE FROM locations WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(id)
	return err
}
