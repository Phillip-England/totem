package handlers

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/extrame/xls"
	"github.com/phillip-england/totem/pkg/data"
	"github.com/phillip-england/vii"
	"github.com/xuri/excelize/v2"
)

type bioEmployeeRow struct {
	FirstName     string
	LastName      string
	TimePunchName string
	Terminated    bool
}

func parseBioEmployeesFromSpreadsheet(reader io.Reader, filename string) ([]bioEmployeeRow, error) {
	rows, err := readRowsFromSpreadsheet(reader, filename)
	if err != nil {
		return nil, err
	}

	headerIndex := map[string]int{}
	for i, header := range rows[0] {
		headerIndex[normalizeHeader(header)] = i
	}

	nameIdx, ok := headerIndex["employee name"]
	if !ok {
		return nil, fmt.Errorf("missing required column: employee name")
	}
	statusIdx := -1
	if idx, ok := headerIndex["employee status"]; ok {
		statusIdx = idx
	}
	termDateIdx := -1
	if idx, ok := headerIndex["termination date"]; ok {
		termDateIdx = idx
	}

	var employees []bioEmployeeRow
	for _, row := range rows[1:] {
		name := cellValue(row, nameIdx)
		first, last, timePunch, ok := splitTimePunchName(name)
		if !ok {
			continue
		}
		status := cellValue(row, statusIdx)
		termDate := cellValue(row, termDateIdx)
		employees = append(employees, bioEmployeeRow{
			FirstName:     first,
			LastName:      last,
			TimePunchName: timePunch,
			Terminated:    isTerminated(status, termDate),
		})
	}

	return employees, nil
}

type birthdateRow struct {
	TimePunchName string
	Birthday      string
}

func parseBirthdatesFromSpreadsheet(reader io.Reader, filename string) ([]birthdateRow, error) {
	rows, err := readRowsFromSpreadsheet(reader, filename)
	if err != nil {
		return nil, err
	}

	headerIndex := map[string]int{}
	for i, header := range rows[0] {
		headerIndex[normalizeHeader(header)] = i
	}

	nameIdx, ok := headerIndex["employee name"]
	if !ok {
		return nil, fmt.Errorf("missing required column: employee name")
	}

	birthIdx := -1
	if idx, ok := headerIndex["birth date"]; ok {
		birthIdx = idx
	}
	if idx, ok := headerIndex["birthdate"]; ok && birthIdx == -1 {
		birthIdx = idx
	}
	if idx, ok := headerIndex["birthday"]; ok && birthIdx == -1 {
		birthIdx = idx
	}
	if birthIdx == -1 {
		return nil, fmt.Errorf("missing required column: birth date")
	}

	var rowsOut []birthdateRow
	for _, row := range rows[1:] {
		name := cellValue(row, nameIdx)
		_, _, timePunch, ok := splitTimePunchName(name)
		if !ok {
			continue
		}
		birthday := cellValue(row, birthIdx)
		if birthday == "" {
			continue
		}
		normalizedBirthday, ok := normalizeBirthday(birthday)
		if !ok {
			continue
		}
		rowsOut = append(rowsOut, birthdateRow{
			TimePunchName: timePunch,
			Birthday:      normalizedBirthday,
		})
	}

	return rowsOut, nil
}

type hsJobRow struct {
	FirstName     string
	LastName      string
	PreferredName string
	Department    string
}

func parseHotSchedulesDepartmentsFromHTML(value string) ([]hsJobRow, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, fmt.Errorf("hot schedules html is required")
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return nil, err
	}

	rows := doc.Find("#stafftable tbody tr")
	if rows.Length() == 0 {
		rows = doc.Find("table#stafftable tr")
	}
	if rows.Length() == 0 {
		rows = doc.Find("table.data-table tbody tr")
	}
	if rows.Length() == 0 {
		return nil, fmt.Errorf("could not find employee table rows")
	}

	var rowsOut []hsJobRow
	rows.Each(func(_ int, row *goquery.Selection) {
		cells := row.Find("td")
		if cells.Length() < 7 {
			return
		}
		nameCell := cells.Eq(1)
		name := strings.TrimSpace(nameCell.Find("a").First().Text())
		if name == "" {
			name = strings.TrimSpace(nameCell.Text())
		}
		name = strings.Join(strings.Fields(name), " ")
		first, last, ok := splitFirstLastFromDisplayName(name)
		if !ok {
			return
		}

		preferred := strings.TrimSpace(cells.Eq(2).Text())
		preferred = strings.Join(strings.Fields(preferred), " ")
		if preferred == "-" {
			preferred = ""
		}

		jobCell := cells.Eq(6)
		jobs := extractHotSchedulesJobs(jobCell)
		if len(jobs) == 0 {
			return
		}

		department, ok := mapDepartmentFromJobs(strings.Join(jobs, " | "))
		if !ok {
			return
		}
		rowsOut = append(rowsOut, hsJobRow{
			FirstName:     first,
			LastName:      last,
			PreferredName: preferred,
			Department:    department,
		})
	})

	if len(rowsOut) == 0 {
		return nil, fmt.Errorf("no mappable employees found in html")
	}

	return rowsOut, nil
}

func normalizeHeader(header string) string {
	return strings.ToLower(strings.TrimSpace(header))
}

func cellValue(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func splitTimePunchName(name string) (string, string, string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", "", "", false
	}

	if strings.Contains(trimmed, ",") {
		parts := strings.SplitN(trimmed, ",", 2)
		last := strings.TrimSpace(parts[0])
		first := strings.TrimSpace(parts[1])
		if first == "" || last == "" {
			return "", "", "", false
		}
		return first, last, canonicalTimePunchName(first, last), true
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", "", "", false
	}
	first := fields[0]
	last := fields[len(fields)-1]
	return first, last, canonicalTimePunchName(first, last), true
}

func canonicalTimePunchName(firstName, lastName string) string {
	return strings.ToLower(strings.TrimSpace(lastName)) + ", " + strings.ToLower(strings.TrimSpace(firstName))
}

func canonicalTimePunchNameFromValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	_, _, timePunch, ok := splitTimePunchName(trimmed)
	if ok {
		return timePunch
	}
	return strings.ToLower(trimmed)
}

func isTerminated(status, terminationDate string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	terminationDate = strings.TrimSpace(terminationDate)
	if terminationDate != "" {
		return true
	}
	return strings.Contains(status, "terminat") || strings.Contains(status, "inactive")
}

func normalizeBirthday(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	dateFormats := []string{
		"2006-01-02",
		"1/2/2006",
		"01/02/2006",
		"1/2/06",
		"01/02/06",
		"1-2-2006",
		"01-02-2006",
		"1-2-06",
		"01-02-06",
	}

	for _, format := range dateFormats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed.Format("2006-01-02"), true
		}
	}

	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed.Format("2006-01-02"), true
	}

	return "", false
}

func splitFirstLastFromDisplayName(name string) (string, string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", "", false
	}

	if strings.Contains(trimmed, ",") {
		parts := strings.SplitN(trimmed, ",", 2)
		last := strings.TrimSpace(parts[0])
		first := strings.TrimSpace(parts[1])
		if first == "" || last == "" {
			return "", "", false
		}
		return first, last, true
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", "", false
	}
	first := fields[0]
	last := strings.Join(fields[1:], " ")
	return first, last, true
}

func normalizeNameKey(first, last string) string {
	first = normalizeFirstName(first)
	last = normalizeLastName(last)
	if first == "" || last == "" {
		return ""
	}
	return last + "|" + first
}

func normalizeFirstName(value string) string {
	value = stripParenthetical(value)
	value = normalizeNameText(value)
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func normalizeLastName(value string) string {
	value = normalizeNameText(value)
	return strings.Join(strings.Fields(value), " ")
}

func normalizeNameText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastWasSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
			lastWasSpace = false
			continue
		}
		if !lastWasSpace {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func stripParenthetical(value string) string {
	var b strings.Builder
	depth := 0
	for _, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func extractHotSchedulesJobs(cell *goquery.Selection) []string {
	var jobs []string
	tooltip := ""
	cell.Find("[tooltip]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if value, ok := s.Attr("tooltip"); ok && strings.TrimSpace(value) != "" {
			tooltip = value
			return false
		}
		return true
	})

	if tooltip != "" {
		decoded := html.UnescapeString(tooltip)
		if doc, err := goquery.NewDocumentFromReader(strings.NewReader(decoded)); err == nil {
			doc.Find("li").Each(func(_ int, li *goquery.Selection) {
				text := strings.Join(strings.Fields(li.Text()), " ")
				if text != "" {
					jobs = append(jobs, text)
				}
			})
		}
	}

	if len(jobs) == 0 {
		text := strings.Join(strings.Fields(cell.Text()), " ")
		if text != "" && text != "-" {
			jobs = append(jobs, text)
		}
	}

	return jobs
}

type timePunchEmployeeTotals struct {
	Name       string
	Department string
	Hours      float64
	Wages      float64
}

type timePunchDepartmentTotals struct {
	Department string
	Hours      float64
	Wages      float64
}

type timePunchSummary struct {
	StartDate          string
	EndDate            string
	DayCount           int
	DepartmentTotals   []timePunchDepartmentTotals
	EmployeeTotals     []timePunchEmployeeTotals
	TotalHours         float64
	OvertimeHours      float64
	TotalWages         float64
	DepartmentHours    float64
	DepartmentWages    float64
	DepartmentHoursAll float64
	WagesWithSalary    float64
	WagesWithPayroll   float64
	PayrollEventsTotal float64
	TotalSales         float64
	Productivity       float64
	SalaryAmount       float64
	SalaryHours        float64
	UnmatchedEmployees int
}

type timePunchReportTotals struct {
	TotalHours    float64
	RegularHours  float64
	OvertimeHours float64
	TotalWages    float64
	RegularWages  float64
	OvertimeWages float64
}

func parseTimePunchReport(text string) (map[string]timePunchEmployeeTotals, timePunchReportTotals, time.Time, time.Time, error) {
	lines := strings.Split(text, "\n")
	var currentName string
	employeeTotals := make(map[string]timePunchEmployeeTotals)
	var startDate time.Time
	var endDate time.Time
	var totals timePunchReportTotals

	dateRangeRe := regexp.MustCompile(`(?i)from\s+\w+,\s+([A-Za-z]{3}\s+\d{1,2},\s+\d{4})\s+through\s+\w+,\s+([A-Za-z]{3}\s+\d{1,2},\s+\d{4})`)
	timeRe := regexp.MustCompile(`\d{1,3}:\d{2}`)
	moneyRe := regexp.MustCompile(`\$\d[\d,]*\.\d{2}`)

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "All Employees Grand Total") {
			timeMatches := timeRe.FindAllString(line, -1)
			moneyMatches := moneyRe.FindAllString(line, -1)
			if len(timeMatches) >= 3 {
				if hours, ok := parseTimePunchHours(timeMatches[0]); ok {
					totals.TotalHours = hours
				}
				if hours, ok := parseTimePunchHours(timeMatches[1]); ok {
					totals.RegularHours = hours
				}
				if hours, ok := parseTimePunchHours(timeMatches[2]); ok {
					totals.OvertimeHours = hours
				}
			}
			if len(moneyMatches) >= 3 {
				if amount, ok := parseTimePunchMoney(moneyMatches[0]); ok {
					totals.RegularWages = amount
				}
				if amount, ok := parseTimePunchMoney(moneyMatches[1]); ok {
					totals.OvertimeWages = amount
				}
				if amount, ok := parseTimePunchMoney(moneyMatches[2]); ok {
					totals.TotalWages = amount
				}
			}
			continue
		}
		if matches := dateRangeRe.FindStringSubmatch(line); len(matches) == 3 {
			if parsed, err := time.Parse("Jan 2, 2006", matches[1]); err == nil {
				startDate = parsed
			}
			if parsed, err := time.Parse("Jan 2, 2006", matches[2]); err == nil {
				endDate = parsed
			}
			continue
		}
		if strings.HasPrefix(line, "Employee Totals") {
			if currentName == "" {
				continue
			}
			timeMatches := timeRe.FindAllString(line, -1)
			moneyMatches := moneyRe.FindAllString(line, -1)
			if len(timeMatches) == 0 || len(moneyMatches) == 0 {
				continue
			}
			hours, ok := parseTimePunchHours(timeMatches[0])
			if !ok {
				continue
			}
			wages, ok := parseTimePunchMoney(moneyMatches[len(moneyMatches)-1])
			if !ok {
				continue
			}
			employeeTotals[currentName] = timePunchEmployeeTotals{
				Name:  currentName,
				Hours: hours,
				Wages: wages,
			}
			continue
		}
		if isTimePunchNameLine(line) {
			currentName = line
		}
	}

	if len(employeeTotals) == 0 {
		return nil, timePunchReportTotals{}, time.Time{}, time.Time{}, fmt.Errorf("no employee totals found in report")
	}
	return employeeTotals, totals, startDate, endDate, nil
}

func isTimePunchNameLine(line string) bool {
	if strings.Contains(line, "Employee Totals") || strings.Contains(line, "All Employees Grand Total") {
		return false
	}
	if strings.HasPrefix(line, "Mon,") || strings.HasPrefix(line, "Tue,") || strings.HasPrefix(line, "Wed,") ||
		strings.HasPrefix(line, "Thu,") || strings.HasPrefix(line, "Fri,") || strings.HasPrefix(line, "Sat,") ||
		strings.HasPrefix(line, "Sun,") || strings.HasPrefix(line, "* ") {
		return false
	}
	return strings.Contains(line, ",")
}

func parseTimePunchHours(value string) (float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, false
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	return float64(hours) + float64(minutes)/60.0, true
}

func parseTimePunchMoney(value string) (float64, bool) {
	clean := strings.ReplaceAll(value, "$", "")
	clean = strings.ReplaceAll(clean, ",", "")
	amount, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, false
	}
	return amount, true
}

func summarizeTimePunchReport(text string, employees []data.Employee) (timePunchSummary, error) {
	employeeTotals, reportTotals, startDate, endDate, err := parseTimePunchReport(text)
	if err != nil {
		return timePunchSummary{}, err
	}
	return summarizeTimePunchReportFromParsed(employeeTotals, reportTotals, startDate, endDate, employees, nil)
}

func summarizeTimePunchReportFromParsed(employeeTotals map[string]timePunchEmployeeTotals, reportTotals timePunchReportTotals, startDate, endDate time.Time, employees []data.Employee, payrollEvents []data.PayrollEvent) (timePunchSummary, error) {

	dayCount := 0
	if !startDate.IsZero() && !endDate.IsZero() && !endDate.Before(startDate) {
		dayCount = int(endDate.Sub(startDate).Hours()/24) + 1
	}

	employeeByKey := make(map[string]data.Employee, len(employees))
	salaryEmployeesByKey := make(map[string]data.Employee)
	for _, emp := range employees {
		key := normalizeNameKey(emp.FirstName, emp.LastName)
		if key != "" {
			employeeByKey[key] = emp
			if emp.AnnualSalary > 0 {
				salaryEmployeesByKey[key] = emp
			}
		}
	}

	departmentTotals := map[string]timePunchDepartmentTotals{}
	var employeeSummary []timePunchEmployeeTotals
	var salaryHours float64
	var salaryAmount float64
	var unmatched int

	for _, totals := range employeeTotals {
		first, last, _, ok := splitTimePunchName(totals.Name)
		department := "TERMINATED"
		salaryEmployee := false
		annualSalary := 0.0
		if ok {
			key := normalizeNameKey(first, last)
			if emp, found := employeeByKey[key]; found {
				department = emp.Department
				salaryEmployee = emp.AnnualSalary > 0
				annualSalary = emp.AnnualSalary
				delete(salaryEmployeesByKey, key)
			} else {
				unmatched++
			}
		} else {
			unmatched++
		}

		employeeSummary = append(employeeSummary, timePunchEmployeeTotals{
			Name:       totals.Name,
			Department: department,
			Hours:      totals.Hours,
			Wages:      totals.Wages,
		})

		if salaryEmployee {
			salaryHours += totals.Hours
			if dayCount > 0 && annualSalary > 0 {
				salaryAmount += (annualSalary / 365.0) * float64(dayCount)
			}
		}

		entry := departmentTotals[department]
		entry.Department = department
		entry.Hours += totals.Hours
		entry.Wages += totals.Wages
		if salaryEmployee && dayCount > 0 && annualSalary > 0 {
			entry.Wages += (annualSalary / 365.0) * float64(dayCount)
		}
		departmentTotals[department] = entry
	}

	if dayCount > 0 {
		for _, emp := range salaryEmployeesByKey {
			department := emp.Department
			prorated := (emp.AnnualSalary / 365.0) * float64(dayCount)
			salaryAmount += prorated
			entry := departmentTotals[department]
			entry.Department = department
			entry.Wages += prorated
			departmentTotals[department] = entry
			employeeSummary = append(employeeSummary, timePunchEmployeeTotals{
				Name:       strings.TrimSpace(emp.LastName + ", " + emp.FirstName),
				Department: department,
				Hours:      0,
				Wages:      prorated,
			})
		}
	}

	if unmatched > 0 {
		if _, ok := departmentTotals["TERMINATED"]; !ok {
			departmentTotals["TERMINATED"] = timePunchDepartmentTotals{
				Department: "TERMINATED",
			}
		}
	}

	var departmentSummary []timePunchDepartmentTotals
	for _, entry := range departmentTotals {
		departmentSummary = append(departmentSummary, entry)
	}
	sort.Slice(departmentSummary, func(i, j int) bool {
		return departmentSummary[i].Department < departmentSummary[j].Department
	})
	sort.Slice(employeeSummary, func(i, j int) bool {
		return employeeSummary[i].Name < employeeSummary[j].Name
	})

	if reportTotals.TotalHours == 0 || reportTotals.TotalWages == 0 {
		for _, totals := range employeeTotals {
			reportTotals.TotalHours += totals.Hours
			reportTotals.TotalWages += totals.Wages
		}
	}

	var departmentHours float64
	var departmentWages float64
	for _, entry := range departmentTotals {
		departmentHours += entry.Hours
		departmentWages += entry.Wages
	}

	var payrollEventsTotal float64
	for _, event := range payrollEvents {
		payrollEventsTotal += event.Amount
	}

	wagesWithSalary := departmentWages
	wagesWithPayroll := wagesWithSalary + payrollEventsTotal
	departmentHoursAll := departmentHours

	summary := timePunchSummary{
		StartDate:          formatDateRange(startDate),
		EndDate:            formatDateRange(endDate),
		DayCount:           dayCount,
		DepartmentTotals:   departmentSummary,
		EmployeeTotals:     employeeSummary,
		TotalHours:         reportTotals.TotalHours,
		OvertimeHours:      reportTotals.OvertimeHours,
		TotalWages:         reportTotals.TotalWages,
		DepartmentHours:    departmentHours,
		DepartmentWages:    departmentWages,
		DepartmentHoursAll: departmentHoursAll,
		WagesWithSalary:    wagesWithSalary,
		WagesWithPayroll:   wagesWithPayroll,
		PayrollEventsTotal: payrollEventsTotal,
		TotalSales:         0,
		Productivity:       0,
		SalaryAmount:       salaryAmount,
		SalaryHours:        salaryHours,
		UnmatchedEmployees: unmatched,
	}
	return summary, nil
}

func formatDateRange(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02")
}

func readRowsFromSpreadsheet(reader io.Reader, filename string) ([][]string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".xls":
		workbook, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
		if err != nil {
			return nil, err
		}
		if workbook.NumSheets() == 0 {
			return nil, fmt.Errorf("no worksheet found")
		}
		if workbook.NumSheets() > 1 {
			return nil, fmt.Errorf("multiple worksheets found; please upload a file with a single sheet")
		}
		rows := workbook.ReadAllCells(100000)
		if len(rows) == 0 {
			return nil, fmt.Errorf("worksheet is empty")
		}
		return rows, nil
	default:
		file, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()

		sheetName := file.GetSheetName(0)
		if sheetName == "" {
			return nil, fmt.Errorf("no worksheet found")
		}

		rows, err := file.GetRows(sheetName)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("worksheet is empty")
		}
		return rows, nil
	}
}

func mapDepartmentFromJobs(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	if lower == "-" {
		return "NONE", true
	}

	mappings := []struct {
		Department string
		Job        string
	}{
		{Department: "PARTNER", Job: "Dispatcher"},
		{Department: "EXECUTIVE", Job: "Mobile Drinks"},
		{Department: "CENTRAL", Job: "Lemons"},
		{Department: "DIRECTOR", Job: "Front Counter Stager"},
		{Department: "BOH", Job: "BOH General"},
		{Department: "FOH", Job: "FOH General"},
	}

	for _, mapping := range mappings {
		if strings.Contains(lower, strings.ToLower(mapping.Job)) {
			return mapping.Department, true
		}
	}

	return "", false
}

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

		user, err := data.AuthenticateUser(username, password)
		if err == nil {
			payload := user.PasswordHash
			if isAdminUser(user) {
				payload = adminSessionPayload()
				if payload == "" {
					payload = user.PasswordHash
				}
			}
			expiresAt := time.Now().Add(24 * time.Hour)
			sessionKey, err := data.CreateSession(user.ID, payload, expiresAt)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			setSessionCookie(w, sessionKey, expiresAt)
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

		err = vii.ExecuteTemplate(w, r, "index.html", data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	app.At("GET /logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie != nil && cookie.Value != "" {
			_ = data.DeleteSessionByKey(cookie.Value)
		}
		clearSessionCookie(w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
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

	app.At("GET /admin/users", func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !isAdminUser(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		users, err := data.GetUsers()
		if err != nil {
			users = []data.User{}
		}
		templateData := struct {
			User    data.User
			Users   []data.User
			Message string
		}{
			User:  user,
			Users: users,
		}
		if err := vii.ExecuteTemplate(w, r, "users.html", templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	app.At("POST /admin/users/update", func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !isAdminUser(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		if password == "" {
			http.Error(w, "Password is required", http.StatusBadRequest)
			return
		}
		if err := data.UpdateUserCredentials(user.ID, username, password); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = data.DeleteSessionsByUserID(user.ID)
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	})

	app.At("POST /admin/users/create", func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !isAdminUser(user) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		username := r.FormValue("username")
		password := r.FormValue("password")
		role := r.FormValue("role")
		if role == "" {
			role = "admin"
		}
		if _, err := data.CreateUser(username, password, role); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
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
		now := time.Now()
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		monthEnd := now
		perfRecords, err := data.GetPerformanceReport(id, monthStart.Format("2006-01-02"), monthEnd.Format("2006-01-02"))
		if err != nil {
			perfRecords = []data.DailyPerformanceRecord{}
		}
		perfSummary := data.CalculateSummary(perfRecords)
		dayCount := now.Day()
		avgSales := 0.0
		avgHours := 0.0
		if dayCount > 0 {
			avgSales = perfSummary.Sales / float64(dayCount)
			avgHours = perfSummary.Hours / float64(dayCount)
		}
		productivity := 0.0
		if perfSummary.Hours > 0 {
			productivity = perfSummary.Sales / perfSummary.Hours
		}

		type weekSummary struct {
			Label        string
			TotalSales   float64
			TotalHours   float64
			Productivity float64
		}
		weekTotals := map[time.Time]*weekSummary{}
		for _, rec := range perfRecords {
			dateVal, err := time.Parse("2006-01-02", rec.Date)
			if err != nil {
				continue
			}
			offset := (int(dateVal.Weekday()) + 6) % 7
			weekStart := time.Date(dateVal.Year(), dateVal.Month(), dateVal.Day()-offset, 0, 0, 0, 0, dateVal.Location())
			if weekStart.Before(monthStart) {
				weekStart = monthStart
			}
			entry, ok := weekTotals[weekStart]
			if !ok {
				weekEnd := weekStart.AddDate(0, 0, 6)
				if weekEnd.After(monthEnd) {
					weekEnd = monthEnd
				}
				entry = &weekSummary{
					Label: weekStart.Format("Jan 2") + "–" + weekEnd.Format("Jan 2"),
				}
				weekTotals[weekStart] = entry
			}
			entry.TotalSales += rec.TotalSales
			entry.TotalHours += rec.TotalHours
		}
		var weeks []weekSummary
		for _, entry := range weekTotals {
			if entry.TotalHours > 0 {
				entry.Productivity = entry.TotalSales / entry.TotalHours
			}
			weeks = append(weeks, *entry)
		}
		sort.Slice(weeks, func(i, j int) bool {
			return weeks[i].Label < weeks[j].Label
		})

		templateData := struct {
			Location      data.CfaLocation
			MonthStart    string
			MonthEnd      string
			MonthSales    float64
			AvgDailySales float64
			AvgDailyHours float64
			Productivity  float64
			WeekSummaries []weekSummary
		}{
			Location:      loc,
			MonthStart:    monthStart.Format("2006-01-02"),
			MonthEnd:      monthEnd.Format("2006-01-02"),
			MonthSales:    perfSummary.Sales,
			AvgDailySales: avgSales,
			AvgDailyHours: avgHours,
			Productivity:  productivity,
			WeekSummaries: weeks,
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

	// Payroll Events Page
	app.At("GET /admin/locations/{id}/payroll", func(w http.ResponseWriter, r *http.Request) {
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

		// Default to last 90 days if no filter
		if startDate == "" && endDate == "" {
			now := time.Now()
			endDate = now.Format("2006-01-02")
			startDate = now.AddDate(0, 0, -90).Format("2006-01-02")
		}

		events, err := data.GetPayrollEventsByLocation(id, startDate, endDate)
		if err != nil {
			events = []data.PayrollEvent{}
		}
		employees, err := data.GetAllEmployeesByLocation(id)
		if err != nil {
			employees = []data.Employee{}
		}

		var totalAmount float64
		for _, e := range events {
			totalAmount += e.Amount
		}

		ranges := getCommonRanges()

		templateData := struct {
			Location    data.CfaLocation
			Events      []data.PayrollEvent
			EventTypes  []string
			Employees   []data.Employee
			StartDate   string
			EndDate     string
			Today       string
			TotalAmount float64
			Ranges      struct{ MonthStart, NinetyStart, YTDStart, Today string }
		}{
			Location:    loc,
			Events:      events,
			EventTypes:  data.PayrollEventTypes,
			Employees:   employees,
			StartDate:   startDate,
			EndDate:     endDate,
			Today:       time.Now().Format("2006-01-02"),
			TotalAmount: totalAmount,
			Ranges:      ranges,
		}
		err = vii.ExecuteTemplate(w, r, "payroll_events.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Create Payroll Event
	app.At("POST /admin/locations/{id}/payroll", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		date := r.FormValue("date")
		employeeIDStr := r.FormValue("employee_id")
		employeeID, err := strconv.Atoi(employeeIDStr)
		if err != nil || employeeID == 0 {
			http.Error(w, "Employee is required", http.StatusBadRequest)
			return
		}
		eventType := r.FormValue("event_type")
		description := r.FormValue("description")
		amountStr := r.FormValue("amount")
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || date == "" || eventType == "" || description == "" {
			http.Error(w, "Invalid input", http.StatusBadRequest)
			return
		}
		err = data.CreatePayrollEvent(id, employeeID, date, eventType, description, amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/payroll", http.StatusSeeOther)
	})

	// Delete Payroll Event
	app.At("POST /admin/locations/{id}/payroll/{eventId}/delete", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		eventIdStr := r.PathValue("eventId")
		eventId, err := strconv.Atoi(eventIdStr)
		if err != nil {
			http.Error(w, "Invalid Event ID", http.StatusBadRequest)
			return
		}
		err = data.DeletePayrollEvent(eventId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/payroll", http.StatusSeeOther)
	})

	// Employees Page
	app.At("GET /admin/locations/{id}/employees", func(w http.ResponseWriter, r *http.Request) {
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

		status := r.URL.Query().Get("status")
		if status == "" {
			status = "active"
		}

		var employees []data.Employee
		switch status {
		case "terminated":
			employees, err = data.GetTerminatedEmployeesByLocation(id)
		case "all":
			employees, err = data.GetAllEmployeesByLocation(id)
		default:
			status = "active"
			employees, err = data.GetActiveEmployeesByLocation(id)
		}
		if err != nil {
			employees = []data.Employee{}
		}

		templateData := struct {
			Location  data.CfaLocation
			Employees []data.Employee
			Status    string
		}{
			Location:  loc,
			Employees: employees,
			Status:    status,
		}
		err = vii.ExecuteTemplate(w, r, "employees.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Create Employee
	app.At("POST /admin/locations/{id}/employees", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		firstName := r.FormValue("first_name")
		lastName := r.FormValue("last_name")
		if firstName == "" || lastName == "" {
			http.Error(w, "First name and last name are required", http.StatusBadRequest)
			return
		}
		err = data.CreateEmployee(id, firstName, lastName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Import Employees from Bio XLSX
	app.At("POST /admin/locations/{id}/employees/import", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("bio_file")
		if err != nil {
			http.Error(w, "Bio XLSX file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		bioEmployees, err := parseBioEmployeesFromSpreadsheet(file, header.Filename)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		existingEmployees, err := data.GetAllEmployeesByLocation(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		existingByTimePunch := make(map[string]data.Employee, len(existingEmployees))
		for _, emp := range existingEmployees {
			key := canonicalTimePunchName(emp.FirstName, emp.LastName)
			if emp.TimePunchName != "" {
				key = canonicalTimePunchNameFromValue(emp.TimePunchName)
			}
			existingByTimePunch[key] = emp
		}

		activeByTimePunch := make(map[string]bioEmployeeRow)
		for _, emp := range bioEmployees {
			if emp.Terminated {
				continue
			}
			activeByTimePunch[emp.TimePunchName] = emp
		}

		terminationDate := time.Now().Format("2006-01-02")

		for key, emp := range activeByTimePunch {
			if existing, ok := existingByTimePunch[key]; ok {
				// If employee was terminated but now appears in active bio, reinstate them
				if existing.Terminated {
					if err := data.ReinstateEmployee(existing.ID); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				if existing.FirstName != emp.FirstName || existing.LastName != emp.LastName {
					err := data.UpdateEmployee(existing.ID, emp.FirstName, emp.LastName, existing.Birthday, existing.Department, existing.AnnualSalary)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				continue
			}
			err := data.CreateEmployee(id, emp.FirstName, emp.LastName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		for _, existing := range existingEmployees {
			key := canonicalTimePunchName(existing.FirstName, existing.LastName)
			if existing.TimePunchName != "" {
				key = canonicalTimePunchNameFromValue(existing.TimePunchName)
			}
			if _, ok := activeByTimePunch[key]; ok {
				continue
			}
			// Only terminate if not already terminated
			if !existing.Terminated {
				if err := data.TerminateEmployee(existing.ID, terminationDate); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Import Employee Birthdates
	app.At("POST /admin/locations/{id}/employees/birthdates/import", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("birthdate_file")
		if err != nil {
			http.Error(w, "Birthdate XLSX file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()

		birthdateRows, err := parseBirthdatesFromSpreadsheet(file, header.Filename)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		existingEmployees, err := data.GetEmployeesByLocation(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		existingByTimePunch := make(map[string]data.Employee, len(existingEmployees))
		for _, emp := range existingEmployees {
			key := canonicalTimePunchName(emp.FirstName, emp.LastName)
			if emp.TimePunchName != "" {
				key = canonicalTimePunchNameFromValue(emp.TimePunchName)
			}
			existingByTimePunch[key] = emp
		}

		for _, row := range birthdateRows {
			existing, ok := existingByTimePunch[row.TimePunchName]
			if !ok {
				continue
			}
			if existing.Birthday == row.Birthday {
				continue
			}
			if err := data.UpdateEmployee(existing.ID, existing.FirstName, existing.LastName, row.Birthday, existing.Department, existing.AnnualSalary); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Import Employee Departments from HotSchedules
	app.At("POST /admin/locations/{id}/employees/departments/import", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		htmlValue := r.FormValue("department_html")
		departmentRows, err := parseHotSchedulesDepartmentsFromHTML(htmlValue)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		existingEmployees, err := data.GetEmployeesByLocation(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		existingByTimePunch := make(map[string]data.Employee, len(existingEmployees))
		existingByNameKey := make(map[string]data.Employee, len(existingEmployees))
		for _, emp := range existingEmployees {
			key := canonicalTimePunchName(emp.FirstName, emp.LastName)
			if emp.TimePunchName != "" {
				key = canonicalTimePunchNameFromValue(emp.TimePunchName)
			}
			existingByTimePunch[key] = emp
			nameKey := normalizeNameKey(emp.FirstName, emp.LastName)
			if nameKey != "" {
				existingByNameKey[nameKey] = emp
			}
		}

		for _, row := range departmentRows {
			primaryKey := normalizeNameKey(row.FirstName, row.LastName)
			preferredKey := ""
			if row.PreferredName != "" {
				preferredKey = normalizeNameKey(row.PreferredName, row.LastName)
			}
			existing, ok := existingByNameKey[primaryKey]
			if !ok && preferredKey != "" {
				existing, ok = existingByNameKey[preferredKey]
			}
			if !ok {
				timePunch := canonicalTimePunchName(row.FirstName, row.LastName)
				existing, ok = existingByTimePunch[timePunch]
			}
			if !ok {
				continue
			}
			if existing.Department == row.Department {
				continue
			}
			if err := data.UpdateEmployee(existing.ID, existing.FirstName, existing.LastName, existing.Birthday, row.Department, existing.AnnualSalary); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Time Punch Summary
	app.At("GET /admin/locations/{id}/timepunch", func(w http.ResponseWriter, r *http.Request) {
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
			Summary  *timePunchSummary
			Error    string
		}{
			Location: loc,
		}
		if err := vii.ExecuteTemplate(w, r, "time_punch_summary.html", templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	app.At("POST /admin/locations/{id}/timepunch", func(w http.ResponseWriter, r *http.Request) {
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
		text := r.FormValue("time_punch_text")
		employees, err := data.GetEmployeesByLocation(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		employeeTotals, reportTotals, startDate, endDate, err := parseTimePunchReport(text)
		if err != nil {
			templateData := struct {
				Location data.CfaLocation
				Summary  *timePunchSummary
				Error    string
			}{
				Location: loc,
				Error:    err.Error(),
			}
			if err := vii.ExecuteTemplate(w, r, "time_punch_summary.html", templateData); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		payrollEvents := []data.PayrollEvent{}
		if !startDate.IsZero() && !endDate.IsZero() && !endDate.Before(startDate) {
			payrollEvents, err = data.GetPayrollEventsByLocation(id, formatDateRange(startDate), formatDateRange(endDate))
			if err != nil {
				payrollEvents = []data.PayrollEvent{}
			}
		}

		summary, err := summarizeTimePunchReportFromParsed(employeeTotals, reportTotals, startDate, endDate, employees, payrollEvents)
		templateData := struct {
			Location data.CfaLocation
			Summary  *timePunchSummary
			Error    string
		}{
			Location: loc,
		}
		if err == nil && !startDate.IsZero() && !endDate.IsZero() && !endDate.Before(startDate) {
			totalSales, err := data.GetTotalSalesByLocation(id, formatDateRange(startDate), formatDateRange(endDate))
			if err == nil {
				summary.TotalSales = totalSales
				if summary.TotalHours > 0 {
					summary.Productivity = totalSales / summary.TotalHours
				}
			}
		}
		if err != nil {
			templateData.Error = err.Error()
		} else {
			templateData.Summary = &summary
		}
		if err := vii.ExecuteTemplate(w, r, "time_punch_summary.html", templateData); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Edit Employee Form
	app.At("GET /admin/locations/{id}/employees/{empId}/edit", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		empIdStr := r.PathValue("empId")
		empId, err := strconv.Atoi(empIdStr)
		if err != nil {
			http.Error(w, "Invalid Employee ID", http.StatusBadRequest)
			return
		}
		loc, err := data.GetLocationByID(id)
		if err != nil {
			http.Error(w, "Location not found", http.StatusNotFound)
			return
		}
		employee, err := data.GetEmployeeByID(empId)
		if err != nil {
			http.Error(w, "Employee not found", http.StatusNotFound)
			return
		}
		templateData := struct {
			Location    data.CfaLocation
			Employee    data.Employee
			Departments []string
		}{
			Location:    loc,
			Employee:    employee,
			Departments: data.Departments,
		}
		err = vii.ExecuteTemplate(w, r, "employee_edit.html", templateData)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Update Employee
	app.At("POST /admin/locations/{id}/employees/{empId}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		empIdStr := r.PathValue("empId")
		empId, err := strconv.Atoi(empIdStr)
		if err != nil {
			http.Error(w, "Invalid Employee ID", http.StatusBadRequest)
			return
		}
		firstName := r.FormValue("first_name")
		lastName := r.FormValue("last_name")
		birthday := r.FormValue("birthday")
		department := r.FormValue("department")
		annualSalaryStr := strings.TrimSpace(r.FormValue("annual_salary"))
		annualSalary := 0.0
		if annualSalaryStr != "" {
			annualSalary, err = strconv.ParseFloat(annualSalaryStr, 64)
			if err != nil || annualSalary < 0 {
				http.Error(w, "Annual salary must be a non-negative number", http.StatusBadRequest)
				return
			}
		}
		if firstName == "" || lastName == "" {
			http.Error(w, "First name and last name are required", http.StatusBadRequest)
			return
		}
		err = data.UpdateEmployee(empId, firstName, lastName, birthday, department, annualSalary)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Delete Employee
	app.At("POST /admin/locations/{id}/employees/{empId}/delete", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		empIdStr := r.PathValue("empId")
		empId, err := strconv.Atoi(empIdStr)
		if err != nil {
			http.Error(w, "Invalid Employee ID", http.StatusBadRequest)
			return
		}
		err = data.DeleteEmployee(empId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Terminate Employee
	app.At("POST /admin/locations/{id}/employees/{empId}/terminate", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		empIdStr := r.PathValue("empId")
		empId, err := strconv.Atoi(empIdStr)
		if err != nil {
			http.Error(w, "Invalid Employee ID", http.StatusBadRequest)
			return
		}
		terminationDate := time.Now().Format("2006-01-02")
		err = data.TerminateEmployee(empId, terminationDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees", http.StatusSeeOther)
	})

	// Reinstate Employee
	app.At("POST /admin/locations/{id}/employees/{empId}/reinstate", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		empIdStr := r.PathValue("empId")
		empId, err := strconv.Atoi(empIdStr)
		if err != nil {
			http.Error(w, "Invalid Employee ID", http.StatusBadRequest)
			return
		}
		err = data.ReinstateEmployee(empId)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/admin/locations/"+idStr+"/employees?status=terminated", http.StatusSeeOther)
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

		// Fetch existing sales data for this date
		existingSales, _ := data.GetSalesByDate(id, today)
		dayPartValues := make(map[string]float64)
		destinationValues := make(map[string]float64)
		for _, sale := range existingSales {
			if sale.Category == "DayPart" {
				dayPartValues[sale.Item] = sale.Amount
			} else if sale.Category == "Destination" {
				destinationValues[sale.Item] = sale.Amount
			}
		}

		templateData := struct {
			Location          data.CfaLocation
			DayParts          []string
			Destinations      []string
			Today             string
			DayPartValues     map[string]float64
			DestinationValues map[string]float64
		}{
			Location:          loc,
			DayParts:          data.DayParts,
			Destinations:      data.Destinations,
			Today:             today,
			DayPartValues:     dayPartValues,
			DestinationValues: destinationValues,
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
				if line == "" {
					continue
				}

				if strings.HasPrefix(line, "Report Totals:") {
					seenReportTotals = true
					continue
				}

				parts := strings.Fields(line)
				if len(parts) < 3 {
					continue
				}

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
			if _, exists := rangeSummary.DayPartPercents[item]; !exists {
				rangeSummary.DayPartPercents[item] = 0
			}
		}
		for _, item := range data.Destinations {
			if _, exists := rangeSummary.DestinationTotals[item]; !exists {
				rangeSummary.DestinationTotals[item] = 0
			}
			if _, exists := rangeSummary.DestinationAverages[item]; !exists {
				rangeSummary.DestinationAverages[item] = 0
			}
			if _, exists := rangeSummary.DestinationPercents[item]; !exists {
				rangeSummary.DestinationPercents[item] = 0
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
		for _, item := range data.DayParts {
			dpTotal += dpMap[item]
		}

		for _, item := range data.DayParts {
			amt := dpMap[item]
			pct := 0.0
			if dpTotal > 0 {
				pct = (amt / dpTotal) * 100
			}
			dayParts = append(dayParts, data.SaleRecord{Item: item, Amount: amt, Percent: pct})
		}

		var destinations []data.SaleRecord
		var destTotal float64
		for _, item := range data.Destinations {
			destTotal += destMap[item]
		}

		for _, item := range data.Destinations {
			amt := destMap[item]
			pct := 0.0
			if destTotal > 0 {
				pct = (amt / destTotal) * 100
			}
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

		// Fetch existing labor data for this date
		existingLabor, _ := data.GetLaborByDate(id, today)

		templateData := struct {
			Location data.CfaLocation
			Today    string
			Existing data.LaborRecord
		}{
			Location: loc,
			Today:    today,
			Existing: existingLabor,
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
							if len(p) != 2 {
								return 0
							}
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
