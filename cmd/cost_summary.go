package cmd

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/ipfs-shipyard/equinix-billing-tools/equinix"
	logging "github.com/ipfs/go-log/v2"
)

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

func (t ReportType) includeUsage(usage equinix.UsageRecord) bool {
	return (t == ReservationsReport && usage.Type == "HardwareReservation") ||
		(t == NonReservationsReport && usage.Type != "HardwareReservation")
}

var log = logging.Logger("equinix-billing-tools")

type CostSummaryT struct {
	equinix       equinix.Equinix
	reportType    ReportType
	onlyGateways  bool
	startDate     time.Time
	endDate       time.Time
	baselineStart time.Time
	baselineEnd   time.Time
}

func CostSummary(eq equinix.Equinix) Command {
	cmd := flag.NewFlagSet("cost_summary", flag.ExitOnError)

	helpF := cmd.Bool("h", false, "Show this help")
	daysF := cmd.Int("d", 1, "Number of days to aggregate (default: 1)")
	endF := cmd.String("e", time.Now().AddDate(0, 0, -2).Format("2006-01-02"), "End date in YYYY-MM-DD format (default: 2 days ago)")
	baselineF := cmd.String("b", "", "Baseline end date in YYYY-MM-DD format (default: day before the start date)")
	typeF := cmd.String("t", "", "Report type: reservations, or blank (meaning everything except reservations) (default: blank)")
	gatewayF := cmd.Bool("g", false, "Only gateways report, splitting between Kubo and LB nodes")

	//
	// Parse command-line arguments
	//

	cmd.Parse(os.Args[2:])

	if *helpF {
		cmd.Usage()
		os.Exit(0)
	}

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

	return CostSummaryT{
		equinix:       eq,
		reportType:    reportType,
		onlyGateways:  *gatewayF,
		startDate:     startTime,
		endDate:       endTime,
		baselineStart: baseStart,
		baselineEnd:   baseEnd,
	}
}

func (s CostSummaryT) Run() {
	//
	// Fetch the usage records for the given date for all projects
	//

	projects, err := s.equinix.GetProjects()
	if err != nil {
		log.Error("Error while getting project list\n%s", err.Error())
		os.Exit(1)
	}

	if s.onlyGateways {
		// Delete all projects from the projects slice except for the gateways project
		projs := make([]equinix.Project, 1, 1)

		for _, p := range projects {
			if p.Name == "gateway" {
				projs[0] = p
			}
		}

		projects = projs
	} else {
		sort.Slice(
			projects,
			func(a, b int) bool {
				return strings.ToUpper(projects[a].Name) < strings.ToUpper(projects[b].Name)
			},
		)
	}

	// Add 1 to the endDate since GetUsages assumes 00:00:00.000
	end := s.endDate.AddDate(0, 0, 1)
	baseEnd := s.baselineEnd.AddDate(0, 0, 1)
	usages, err := s.equinix.GetUsages(s.startDate, end, projects)
	if err != nil {
		log.Error("Error while getting usages\n%s", err.Error())
		os.Exit(1)
	}
	baseline, err := s.equinix.GetUsages(s.baselineStart, baseEnd, projects)
	if err != nil {
		log.Error("Error while getting usages\n%s", err.Error())
		os.Exit(1)
	}

	if s.onlyGateways {
		usages = splitGateways(usages)
		baseline = splitGateways(baseline)
	}

	//
	// Summarize the usage records
	//

	// Summarize by project
	perProjectSummary, totals := summarize(s.reportType, baseline, usages)

	fmt.Printf("%-15.15s %11s %11s\n", "Project", s.baselineEnd.Format("2006-01-02"), s.endDate.Format("2006-01-02"))
	p := message.NewPrinter(language.English)
	for project, summary := range perProjectSummary {
		p.Printf(
			"%-15.15s %11.2f %11.2f %+7.2f%%\n",
			project,
			summary.BaseTotal,
			summary.Total,
			100.0*(summary.Total-summary.BaseTotal)/summary.BaseTotal,
		)

	}

	p.Printf(
		"%-15.15s %11.2f %11.2f %+7.2f%%\n",
		"Total",
		totals.BaseTotal,
		totals.Total,
		100.0*(totals.Total-totals.BaseTotal)/totals.BaseTotal,
	)
}

func splitGateways(usages map[string][]equinix.UsageRecord) map[string][]equinix.UsageRecord {
	gateways := usages["gateway"]
	usages = make(map[string][]equinix.UsageRecord)

	// Split usages between Kubo and LB nodes
	for _, u := range gateways {
		var k string

		if strings.HasPrefix(u.Name, "ipfs-") || (u.Type == "HardwareReservation" && strings.Contains(u.Plan, "medium")) {
			k = "gateway-kubo"
		} else if strings.HasPrefix(u.Name, "gateway-") || (u.Type == "HardwareReservation" && strings.Contains(u.Plan, "small")) {
			k = "gateway-lb"
		}

		usages[k] = append(usages[k], u)
	}

	return usages
}

func summarize(
	reportType ReportType,
	baseline map[string][]equinix.UsageRecord,
	usages map[string][]equinix.UsageRecord,
) (map[string]SummaryRecord, SummaryRecord) {
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

		perProjectSummary[project] = summary
	}

	return perProjectSummary, totals
}
