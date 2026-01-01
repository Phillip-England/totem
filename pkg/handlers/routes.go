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
		
		        if rawText := r.FormValue("raw_text"); rawText != "" {
		            // Parsing Logic for Raw Text
		            lines := strings.Split(rawText, "\n")
		            
		            dpMap := make(map[string]float64)
		            destMap := make(map[string]float64)
		            
		            seenReportTotals := false
		
		            parseMoney := func(s string) float64 {
		                s = strings.ReplaceAll(s, "$", "")
		                s = strings.ReplaceAll(s, ",", "")
		                val, _ := strconv.ParseFloat(s, 64)
		                return val
		            }
		
		            			// Mapping for report names to system names
		            			destMapping := map[string]string{
		            				"CARRY OUT":    "Carry Out",
		            				"DELIVERY":     "Catering Delivery",
		            				"PICKUP":       "Catering Pickup",
		            				"DINE IN":      "Dine-In",
		            				"DRIVE THRU":   "Drive-Thru",
		            				"M-CARRYOUT":   "Mobile Carryout",
		            				"M-DINEIN":     "Mobile Dine-In",
		            				"M-DRIVE-THRU": "Mobile Drive-Thru",
		            				"ON DEMAND":    "Third-Party Delivery",
		            			}
		            
		            			for _, line := range lines {
		            				line = strings.TrimSpace(line)
		            				if line == "" { continue }
		            
		            				if strings.HasPrefix(line, "Report Totals:") {
		            					seenReportTotals = true
		            					continue
		            				}
		            				
		            				parts := strings.Fields(line)
		            				if len(parts) < 3 { continue }
		            
		            				// Check Day Parts
		            				if len(parts) >= 5 && parts[1] == "-" {
		            					dpName := parts[2]
		            					if contains(data.DayParts, dpName) {
		            						sales := parseMoney(parts[4])
		            						dpMap[dpName] += sales
		            					}
		            				}
		            
		            				// Check Destinations
		            				if !seenReportTotals {
		            					// Try to match start of line against keys in destMapping
		            					// We iterate keys, check prefix.
		            					// Keys with spaces (CARRY OUT) need checking.
		            					// Note: "M-..." are single tokens.
		            					
		            					matchedKey := ""
		            					for key := range destMapping {
		            						if strings.HasPrefix(line, key) {
		            							// Prefer longer match? (e.g. CARRY OUT vs CARRY)
		            							if len(key) > len(matchedKey) {
		            								matchedKey = key
		            							}
		            						}
		            					}
		            
		            					if matchedKey != "" {
		            						systemName := destMapping[matchedKey]
		            						// Format: KEY count sales ...
		            						// We need to parse sales which is usually the token AFTER the count.
		            						// Count is after the key.
		            						// Let's tokenize the line again based on the key length or just use Fields.
		            						
		            						// CARRY OUT 200 1,635.31
		            						// Parts: CARRY, OUT, 200, 1,635.31 -> Index 3
		            						// DELIVERY 1 441.50
		            						// Parts: DELIVERY, 1, 441.50 -> Index 2
		            						
		            						keyParts := strings.Fields(matchedKey)
		            						salesIndex := len(keyParts) + 1 // Key tokens + 1 (Count) -> Next is Sales
		            						
		            						if len(parts) > salesIndex {
		            							sales := parseMoney(parts[salesIndex])
		            							destMap[systemName] += sales
		            						}
		            					}
		            				}
		            			}		
		            // Convert Maps to Records
		            for dp, amt := range dpMap {
		                records = append(records, data.SaleRecord{
		                    LocationID: id,
		                    Date:       date,
		                    Category:   "DayPart",
		                    Item:       dp,
		                    Amount:     amt,
		                })
		            }
		            for dest, amt := range destMap {
		                records = append(records, data.SaleRecord{
		                    LocationID: id,
		                    Date:       date,
		                    Category:   "Destination",
		                    Item:       dest,
		                    Amount:     amt,
		                })
		            }
		
		        } else {
		            // Manual Entry Fallback
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

		ranges := getCommonRanges()

		templateData := struct {
			Location       data.CfaLocation
			StartDate      string
			EndDate        string
			DailySummaries []data.DailySummary
			RangeSummary   data.RangeSummary
			Ranges         interface{}
		}{
			Location:       loc,
			StartDate:      startDate,
			EndDate:        endDate,
			DailySummaries: dailySummaries,
			RangeSummary:   rangeSummary,
			Ranges:         ranges,
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
		// First pass: Calculate totals (already done implicitly, but safer to re-sum if we were skipping 0s, but we aren't)
		for _, item := range data.DayParts { dpTotal += dpMap[item] }

		for _, item := range data.DayParts {
			amt := dpMap[item]
			pct := 0.0
			if dpTotal > 0 { pct = (amt / dpTotal) * 100 }
			dayParts = append(dayParts, data.SaleRecord{Item: item, Amount: amt, Percent: pct})
		}

		var destinations []data.SaleRecord
		var destTotal float64
		for _, item := range data.Destinations { destTotal += destMap[item] }

		for _, item := range data.Destinations {
			amt := destMap[item]
			pct := 0.0
			if destTotal > 0 { pct = (amt / destTotal) * 100 }
			destinations = append(destinations, data.SaleRecord{Item: item, Amount: amt, Percent: pct})
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
		rawText := r.FormValue("raw_text")
		
		var regular, overtime, regularWages, overtimeWages float64

		if rawText != "" {
			// Parse Raw Text
			lines := strings.Split(rawText, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "All Employees Grand Total") {
					parts := strings.Fields(line)
					// Expected parts:
					// 0: All
					// 1: Employees
					// 2: Grand
					// 3: Total
					// 4: Total Time (e.g. 427:37) - Ignored for breakdown, but good check
					// 5: Regular Hours (e.g. 427:34)
					// 6: Regular Wages (e.g. $6,328.40)
					// 7: OT Hours (e.g. 0:00)
					// 8: OT Wages (e.g. $0.00)
					// 9: Total Wages (e.g. $6,328.40)

					if len(parts) >= 9 {
						parseHours := func(s string) float64 {
							p := strings.Split(s, ":")
							if len(p) != 2 { return 0 }
							h, _ := strconv.Atoi(p[0])
							m, _ := strconv.Atoi(p[1])
							return float64(h) + float64(m)/60.0
						}
						parseMoney := func(s string) float64 {
							s = strings.ReplaceAll(s, "$", "")
							s = strings.ReplaceAll(s, ",", "")
							val, _ := strconv.ParseFloat(s, 64)
							return val
						}

						regular = parseHours(parts[5])
						regularWages = parseMoney(parts[6])
						overtime = parseHours(parts[7])
						overtimeWages = parseMoney(parts[8])
					}
					break
				}
			}
		} else {
			// Manual Entry Fallback
			regStr := r.FormValue("regular")
			otStr := r.FormValue("overtime")
			regWageStr := r.FormValue("regular_wages")
			otWageStr := r.FormValue("overtime_wages")

			regular, _ = strconv.ParseFloat(regStr, 64)
			overtime, _ = strconv.ParseFloat(otStr, 64)
			regularWages, _ = strconv.ParseFloat(regWageStr, 64)
			overtimeWages, _ = strconv.ParseFloat(otWageStr, 64)
		}

		if date != "" {
			err := data.SaveLabor(id, date, regular, overtime, regularWages, overtimeWages)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin/locations/"+idStr, http.StatusSeeOther)
	})

		// Labor History (Consolidated Performance View)
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
	
					records, err := data.GetPerformanceReport(id, startDate, endDate)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
			
									// Calculate Range Summary
									summary := data.CalculateSummary(records)
							
									ranges := getCommonRanges()
							
									templateData := struct {
										Location  data.CfaLocation
										Records   []data.DailyPerformanceRecord
										StartDate string
										EndDate   string
										Ranges    interface{}
										Summary   data.PerformanceSummary
									}{
										Location:  loc,
										Records:   records,
										StartDate: startDate,
										EndDate:   endDate,
										Ranges:    ranges,
										Summary:   summary,
									}
							
									err = vii.ExecuteTemplate(w, r, "labor_history.html", templateData)
									if err != nil {
										http.Error(w, err.Error(), http.StatusInternalServerError)
									}
								})
							
								// API: Performance Summary
								app.At("GET /api/locations/{id}/performance", func(w http.ResponseWriter, r *http.Request) {
									idStr := r.PathValue("id")
									id, err := strconv.Atoi(idStr)
									if err != nil {
										app.JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
										return
									}
							
									startDate := r.URL.Query().Get("start")
									endDate := r.URL.Query().Get("end")
							
									// Default to today if no range provided? Or last 90?
									// For API, explicit or empty (all) is usually better, but let's match the UI behavior for consistency if not specified.
									// Actually, standard API: if not specified, maybe just today?
									// Let's use the same default: 90 days.
									if startDate == "" && endDate == "" {
										now := time.Now()
										endDate = now.Format("2006-01-02")
										startDate = now.AddDate(0, 0, -90).Format("2006-01-02")
									}
							
									records, err := data.GetPerformanceReport(id, startDate, endDate)
									if err != nil {
										app.JSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
										return
									}
							
									summary := data.CalculateSummary(records)
		app.JSON(w, http.StatusOK, map[string]interface{}{
			"summary":   summary,
			"records":   records,
			"startDate": startDate,
			"endDate":   endDate,
		})
	})
}

func getCommonRanges() struct{ MonthStart, NinetyStart, YTDStart, Today string } {
			now := time.Now()
			today := now.Format("2006-01-02")
		
			// Current Month
			monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		
			// 90 Days
			ninetyStart := now.AddDate(0, 0, -90).Format("2006-01-02")
		
			// YTD
			ytdStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
		
				return struct{ MonthStart, NinetyStart, YTDStart, Today string }{
					MonthStart:  monthStart,
					NinetyStart: ninetyStart,
					YTDStart:    ytdStart,
					Today:       today,
				}
			}
			
			// Helper to check slice containment
			func contains(slice []string, val string) bool {
				for _, item := range slice {
					if item == val {
						return true
					}
				}
				return false
			}
