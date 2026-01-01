package handlers

import (
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/phillip-england/totem/pkg/data"
	"github.com/phillip-england/vii"
)

func RegisterRoutes(app *vii.App) {
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
		locations, err := data.GetAllLocations()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		templateData := struct {
			Locations []data.CfaLocation
		}{
			Locations: locations,
		}

		err = vii.ExecuteTemplate(w, r, "admin.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// View Location Details
	app.At("GET /admin/locations/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}
		templateData := struct {
			Location data.CfaLocation
		}{
			Location: loc,
		}
		err = vii.ExecuteTemplate(w, r, "location_details.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Create Location
	app.At("POST /admin/locations", func(w http.ResponseWriter, r *http.Request) {
		name := r.FormValue("name")
		number := r.FormValue("number")
		if name != "" && number != "" {
			err := data.CreateLocation(name, number)
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
		err = data.DeleteLocation(id)
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
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}
		templateData := struct {
			Location data.CfaLocation
		}{
			Location: loc,
		}
		err = vii.ExecuteTemplate(w, r, "edit_location.html", templateData)
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
		err = data.UpdateLocation(id, name, number)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	})

	// Sales Form
	app.At("GET /admin/locations/{id}/sales/new", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}

		today := r.URL.Query().Get("date")
		if today == "" {
			today = time.Now().Format("2006-01-02")
		}

		templateData := struct {
			Location     data.CfaLocation
			DayParts     []string
			Destinations []string
			Today        string
		}{
			Location:     loc,
			DayParts:     data.DayParts,
			Destinations: data.Destinations,
			Today:        today,
		}
		err = vii.ExecuteTemplate(w, r, "sales_form.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Save Sales
	app.At("POST /admin/locations/{id}/sales", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		
		err = r.ParseForm()
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		date := r.FormValue("date")
		if date == "" {
			http.Error(w, "Date is required", http.StatusBadRequest)
			return
		}

		var records []data.SaleRecord

		// Iterate over all form values to find sales inputs
		for key, values := range r.Form {
			// Check for DayPart inputs
			if strings.HasPrefix(key, "daypart|") {
				parts := strings.Split(key, "|")
				if len(parts) != 2 {
					continue
				}
				item := parts[1]
				amountStr := values[0]
				if amountStr == "" {
					continue
				}
				amount, err := strconv.ParseFloat(amountStr, 64)
				if err != nil {
					continue
				}
				records = append(records, data.SaleRecord{
					LocationID: id,
					Date:       date,
					Category:   "DayPart",
					Item:       item,
					Amount:     amount,
				})
			}

			// Check for Destination inputs
			if strings.HasPrefix(key, "destination|") {
				parts := strings.Split(key, "|")
				if len(parts) != 2 {
					continue
				}
				item := parts[1]
				amountStr := values[0]
				if amountStr == "" {
					continue
				}
				amount, err := strconv.ParseFloat(amountStr, 64)
				if err != nil {
					continue
				}
				records = append(records, data.SaleRecord{
					LocationID: id,
					Date:       date,
					Category:   "Destination",
					Item:       item,
					Amount:     amount,
				})
			}
		}

		if len(records) > 0 {
			err = data.SaveSalesBatch(id, date, records)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin/locations/"+idStr, http.StatusSeeOther)
	})

	// Sales History List (with Range Filter)
	app.At("GET /admin/locations/{id}/sales/history", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}

		startDate := r.URL.Query().Get("start")
		endDate := r.URL.Query().Get("end")

		// Default to last 90 days if no filter provided
		if startDate == "" && endDate == "" {
			now := time.Now()
			endDate = now.Format("2006-01-02")
			startDate = now.AddDate(0, 0, -90).Format("2006-01-02")
		}

		dailySummaries, rangeSummary, err := data.GetSalesSummaries(id, startDate, endDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Ensure all categories are present in rangeSummary for consistent display
		for _, item := range data.DayParts {
			if _, exists := rangeSummary.DayPartTotals[item]; !exists {
				rangeSummary.DayPartTotals[item] = 0
			}
			if _, exists := rangeSummary.DayPartAverages[item]; !exists {
				rangeSummary.DayPartAverages[item] = 0
			}
		}
		for _, item := range data.Destinations {
			if _, exists := rangeSummary.DestinationTotals[item]; !exists {
				rangeSummary.DestinationTotals[item] = 0
			}
			if _, exists := rangeSummary.DestinationAverages[item]; !exists {
				rangeSummary.DestinationAverages[item] = 0
			}
		}

		// Sort Daily Summaries by Date DESC
		sort.Slice(dailySummaries, func(i, j int) bool {
			return dailySummaries[i].Date > dailySummaries[j].Date
		})

		templateData := struct {
			Location       data.CfaLocation
			StartDate      string
			EndDate        string
			DailySummaries []data.DailySummary
			RangeSummary   data.RangeSummary
		}{
			Location:       loc,
			StartDate:      startDate,
			EndDate:        endDate,
			DailySummaries: dailySummaries,
			RangeSummary:   rangeSummary,
		}

		err = vii.ExecuteTemplate(w, r, "sales_list.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Daily Sales Detail
	app.At("GET /admin/locations/{id}/sales/date/{date}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		dateStr := r.PathValue("date")

		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}

		sales, err := data.GetSalesByDate(id, dateStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Initialize maps for easy lookup
		dpMap := make(map[string]float64)
		destMap := make(map[string]float64)

		for _, s := range sales {
			if s.Category == "DayPart" {
				dpMap[s.Item] = s.Amount
			} else if s.Category == "Destination" {
				destMap[s.Item] = s.Amount
			}
		}

		// Construct final slices using predefined order and ensuring all items exist
		var dayParts []data.SaleRecord
		var dpTotal float64
		for _, item := range data.DayParts {
			amt := dpMap[item]
			dayParts = append(dayParts, data.SaleRecord{Item: item, Amount: amt})
			dpTotal += amt
		}

		var destinations []data.SaleRecord
		var destTotal float64
		for _, item := range data.Destinations {
			amt := destMap[item]
			destinations = append(destinations, data.SaleRecord{Item: item, Amount: amt})
			destTotal += amt
		}

		templateData := struct {
			Location         data.CfaLocation
			Date             string
			DayParts         []data.SaleRecord
			Destinations     []data.SaleRecord
			DayPartTotal     float64
			DestinationTotal float64
		}{
			Location:         loc,
			Date:             dateStr,
			DayParts:         dayParts,
			Destinations:     destinations,
			DayPartTotal:     dpTotal,
			DestinationTotal: destTotal,
		}

		err = vii.ExecuteTemplate(w, r, "sales_day_detail.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Labor Form
	app.At("GET /admin/locations/{id}/labor/new", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}

		today := r.URL.Query().Get("date")
		if today == "" {
			today = time.Now().Format("2006-01-02")
		}

		templateData := struct {
			Location data.CfaLocation
			Today    string
		}{
			Location: loc,
			Today:    today,
		}
		err = vii.ExecuteTemplate(w, r, "labor_form.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Save Labor
	app.At("POST /admin/locations/{id}/labor", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		date := r.FormValue("date")
		regStr := r.FormValue("regular")
		otStr := r.FormValue("overtime")

		regular, _ := strconv.ParseFloat(regStr, 64)
		overtime, _ := strconv.ParseFloat(otStr, 64)

		if date != "" {
			err := data.SaveLabor(id, date, regular, overtime)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin/locations/"+idStr, http.StatusSeeOther)
	})

	// Labor History
	app.At("GET /admin/locations/{id}/labor/history", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}

		startDate := r.URL.Query().Get("start")
		endDate := r.URL.Query().Get("end")

		// Default to last 90 days if no filter provided
		if startDate == "" && endDate == "" {
			now := time.Now()
			endDate = now.Format("2006-01-02")
			startDate = now.AddDate(0, 0, -90).Format("2006-01-02")
		}

		records, err := data.GetLaborSummaries(id, startDate, endDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		templateData := struct {
			Location  data.CfaLocation
			Records   []data.LaborRecord
			StartDate string
			EndDate   string
		}{
			Location:  loc,
			Records:   records,
			StartDate: startDate,
			EndDate:   endDate,
		}

		err = vii.ExecuteTemplate(w, r, "labor_history.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Productivity Report
	app.At("GET /admin/locations/{id}/productivity", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}

		startDate := r.URL.Query().Get("start")
		endDate := r.URL.Query().Get("end")

		// Default to last 90 days if no filter provided
		if startDate == "" && endDate == "" {
			now := time.Now()
			endDate = now.Format("2006-01-02")
			startDate = now.AddDate(0, 0, -90).Format("2006-01-02")
		}

		report, err := data.GetProductivityReport(id, startDate, endDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sort by Date DESC
		sort.Slice(report, func(i, j int) bool {
			return report[i].Date > report[j].Date
		})

		templateData := struct {
			Location  data.CfaLocation
			Report    []data.ProductivityRecord
			StartDate string
			EndDate   string
		}{
			Location:  loc,
			Report:    report,
			StartDate: startDate,
			EndDate:   endDate,
		}

		err = vii.ExecuteTemplate(w, r, "productivity_history.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}
