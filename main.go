package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type Project struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type Projects struct {
	Projects []Project `json:"projects"`
}

type UsageRecord struct {
	Date     string
	Metro    string  `json:"metro"`
	Plan     string  `json:"plan"`
	Type     string  `json:"type"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Total    float64 `json:"total"`
}

type UsageRecords struct {
	Usages []UsageRecord `json:"usages"`
}

type SummaryRecord struct {
	Price        float64
	Quantity     float64
	Total        float64
	BasePrice    float64
	BaseQuantity float64
	BaseTotal    float64
}

type ReportType string

// Report type
const (
	ReservationsReport    ReportType = "reservations" // Display hardware reservations
	NonReservationsReport ReportType = ""             // Display everything except hardware reservations
)

func (t ReportType) includeUsage(usage UsageRecord) bool {
	return (t == ReservationsReport && usage.Type == "HardwareReservation") ||
		(t == NonReservationsReport && usage.Type != "HardwareReservation")
}

var log = logging.Logger("equinix-scraper")

func main() {
	equinixToken := os.Getenv("EQUINIX_TOKEN")

	if len(equinixToken) == 0 {
		log.Error("Please set the EQUINIX_TOKEN environment variable")
		os.Exit(1)
	}

	//
	// Parse command-line arguments
	//

	daysF := flag.Int("d", 1, "Number of days to aggregate (default: 1)")
	typeF := flag.String("t", "", "Report type: reservations, or blank (meaning everything except reservations) (default: blank)")
	endF := flag.String("e", time.Now().AddDate(0, 0, -2).Format("2006-01-02"), "End date in YYYY-MM-DD format (default: 2 days ago)")
	baselineF := flag.String("b", "", "Baseline end date in YYYY-MM-DD format (default: day before the start date)")

	flag.Parse()

	days := *daysF

	var reportType ReportType

	if *typeF == "reservations" {
		reportType = ReservationsReport
	} else if *typeF == "" {
		reportType = NonReservationsReport
	} else {
		log.Errorf("Invalid report type, only valid types are \"reservations\" or <blank>")
		os.Exit(1)
	}

	var endTime time.Time
	var err error

	endTime, err = time.Parse("2006-01-02", *endF)
	if err != nil {
		log.Errorf("Invalid date %s, it must be in YYYY-MM-DD format: %s", *endF, err.Error())
		os.Exit(1)
	}

	startTime := endTime.AddDate(0, 0, -days+1)

	var baseEnd time.Time

	if *baselineF == "" {
		baseEnd = startTime.AddDate(0, 0, -1)
	} else {
		baseEnd, err = time.Parse("2006-01-02", *baselineF)
		if err != nil {
			log.Errorf("Invalid date %s, it must be in YYYY-MM-DD format: %s", *baselineF, err.Error())
			os.Exit(1)
		}
	}

	baseStart := baseEnd.AddDate(0, 0, -days+1)

	//
	// Fetch the usage records for the given date for all projects
	//

	projects, err := getProjects(equinixToken)
	if err != nil {
		log.Error("Error while getting project list\n%s", err.Error())
		os.Exit(1)
	}

	sort.Slice(
		projects,
		func(a, b int) bool {
			return strings.ToUpper(projects[a].Name) < strings.ToUpper(projects[b].Name)
		},
	)

	usages, err := getUsages(equinixToken, startTime, endTime, projects)
	if err != nil {
		log.Error("Error while getting usages\n%s", err.Error())
		os.Exit(1)
	}
	baseline, err := getUsages(equinixToken, baseStart, baseEnd, projects)
	if err != nil {
		log.Error("Error while getting usages\n%s", err.Error())
		os.Exit(1)
	}

	//
	// Summarize the usage records
	//

	// Summarize by project, disregarding instance reservations

	perProjectSummary := make(map[string]SummaryRecord)

	totals := SummaryRecord{
		Price:        0,
		Quantity:     0,
		Total:        0,
		BasePrice:    0,
		BaseQuantity: 0,
		BaseTotal:    0,
	}

	for project, projectUsages := range usages {
		summary := SummaryRecord{
			Price:        0,
			Quantity:     0,
			Total:        0,
			BasePrice:    0,
			BaseQuantity: 0,
			BaseTotal:    0,
		}
		baseUsages := baseline[project]

		for _, usage := range projectUsages {
			if reportType.includeUsage(usage) {
				summary.Price += usage.Price
				summary.Quantity += usage.Quantity
				summary.Total += usage.Total
			}
		}

		for _, usage := range baseUsages {
			if reportType.includeUsage(usage) {
				summary.BasePrice += usage.Price
				summary.BaseQuantity += usage.Quantity
				summary.BaseTotal += usage.Total
			}
		}

		totals.Price += summary.Price
		totals.Quantity += summary.Quantity
		totals.Total += summary.Total
		totals.BasePrice += summary.BasePrice
		totals.BaseQuantity += summary.BaseQuantity
		totals.BaseTotal += summary.BaseTotal

		perProjectSummary[project.Name] = summary
	}

	fmt.Printf("%-15.15s %11s %11s\n", "Project", endTime.Format("2006-01-02"), baseEnd.Format("2006-01-02"))
	p := message.NewPrinter(language.English)
	for _, project := range projects {
		summary := perProjectSummary[project.Name]

		p.Printf(
			"%-15.15s %11.2f %11.2f %+7.2f%%\n",
			project.Name,
			summary.Total,
			summary.BaseTotal,
			100.0*(summary.Total-summary.BaseTotal)/summary.BaseTotal,
		)

	}

	p.Printf(
		"%-15.15s %11.2f %11.2f %+7.2f%%\n",
		"Total",
		totals.Total,
		totals.BaseTotal,
		100.0*(totals.Total-totals.BaseTotal)/totals.BaseTotal,
	)
}

func getProjects(token string) ([]Project, error) {
	client := &http.Client{}
	req, err := http.NewRequest(
		"GET",
		"https://api.equinix.com/metal/v1/projects?page=1&per_page=1000&include=id,name",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("Error while creating HTTP request: %w", err)
	}

	req.Header.Add("X-Auth-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error while making the HTTP request: %w", err)
	}
	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error while reading the response body: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(
			"HTTP error.\nStatus code: %d\nResponse body: %s",
			resp.StatusCode,
			string(bytes),
		)
	}

	var projects Projects

	err = json.Unmarshal(bytes, &projects)
	if err != nil {
		return nil, fmt.Errorf("Error while unmarshaling JSON response: %w", err)
	}

	return projects.Projects, nil
}

func getUsages(token string, startDate time.Time, endDate time.Time, projects []Project) (map[Project][]UsageRecord, error) {
	client := &http.Client{}
	usages := make(map[Project][]UsageRecord)

	for _, project := range projects {
		uri := fmt.Sprintf(
			"https://api.equinix.com/metal/v1/projects/%s/usages?created[after]=%sT00:00:00&created[before]=%sT23:59:59.999",
			project.Id,
			startDate.Format("2006-01-02"),
			endDate.Format("2006-01-02"),
		)
		req, err := http.NewRequest(
			"GET",
			uri,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("Error while creating HTTP request for project %s: %w", project.Id, err)
		}

		req.Header.Add("X-Auth-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Error while making the HTTP request for project %s: %w", project.Id, err)
		}
		defer resp.Body.Close()

		bytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("Error while reading the response body for project %s: %w", project.Id, err)
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf(
				"HTTP error for project %s.\nStatus code: %d\nResponse body: %s",
				project.Id,
				resp.StatusCode,
				string(bytes),
			)
		}

		var records UsageRecords

		err = json.Unmarshal(bytes, &records)
		if err != nil {
			return nil, fmt.Errorf("Error while unmarshaling JSON response for project %s: %w", project.Id, err)
		}

		usages[project] = records.Usages
	}

	return usages, nil
}
