package cmd

import (
	"context"
	"flag"
	"os"
	"time"

	bq "cloud.google.com/go/bigquery"
	"github.com/ipfs-shipyard/equinix-billing-tools/common"
	"github.com/ipfs-shipyard/equinix-billing-tools/equinix"
)

type usageRecord struct {
	startTime time.Time
	endTime   time.Time
	project   string
	metro     string
	plan      string
	tpe       string
	name      string
	price     float64
	quantity  float64
	total     float64
}

// Save implements the ValueSaver interface. We disable best-effort deduplication to get better throughput
func (r usageRecord) Save() (map[string]bq.Value, string, error) {
	return map[string]bq.Value{
		"start_time": r.startTime,
		"end_time":   r.endTime,
		"project":    r.project,
		"metro":      r.metro,
		"plan":       r.plan,
		"type":       r.tpe,
		"name":       r.name,
		"price":      r.price,
		"quantity":   r.quantity,
		"total":      r.total,
	}, bq.NoDedupeID, nil
}

type UploadToBigqueryT struct {
	equinix   equinix.Equinix
	startTime time.Time
	endTime   time.Time
	projectId string
	datasetId string
	tableId   string
}

func UploadToBigquery(eq equinix.Equinix) Command {
	cmd := flag.NewFlagSet("bigquery", flag.ExitOnError)

	helpF := cmd.Bool("h", false, "Show this help")
	startF := cmd.String("s", time.Now().AddDate(0, 0, -2).Format(common.ISO8601_FORMAT), "Start time in ISO8601 format")
	secondsF := cmd.Int64("i", 86400, "Time interval in seconds")
	projectIdF := cmd.String("p", "", "Project ID (mandatory)")
	datasetIdF := cmd.String("d", "", "Dataset ID (mandatory)")
	tableIdF := cmd.String("t", "", "Table ID (mandatory)")

	cmd.Parse(os.Args[2:])

	if *helpF {
		cmd.Usage()
		os.Exit(0)
	}

	var startTime time.Time
	var err error

	startTime, err = common.ParsePartialIsoTime(*startF)
	if err != nil {
		log.Errorf("Invalid end time %s, it must be in ISO8601 format: %s", *startF, err.Error())
		os.Exit(1)
	}

	endTime := startTime.Add(time.Duration(*secondsF) * time.Second)

	log.Infof("Inserting from %v to %v", startTime, endTime)

	// TODO Validate project.dataset.table
	// TODO Dockerfile

	return UploadToBigqueryT{
		equinix:   eq,
		startTime: startTime,
		endTime:   endTime,
		projectId: *projectIdF,
		datasetId: *datasetIdF,
		tableId:   *tableIdF,
	}
}

func (up UploadToBigqueryT) Run() {
	projects, err := up.equinix.GetProjects()
	if err != nil {
		log.Error("Error while getting project list\n%s", err.Error())
		os.Exit(1)
	}

	projUsages, err := up.equinix.GetUsages(up.startTime, up.endTime, projects)
	if err != nil {
		log.Error("Error while getting usages\n%s", err.Error())
		os.Exit(1)
	}

	ctx := context.Background()
	client, err := bq.NewClient(ctx, up.projectId)
	if err != nil {
		log.Error("Error while creating BigQuery client\n%s", err.Error())
		os.Exit(1)
	}
	defer client.Close()
	inserter := client.Dataset(up.datasetId).Table(up.tableId).Inserter()

	for project, usages := range projUsages {
		items := make([]usageRecord, 0, len(usages))
		for _, u := range usages {
			// Equinix has some weirdness regarding "plan" vs "type"
			if u.Plan == "Outbound Bandwidth" {
				u.Type = u.Plan
			}

			// Equinix returns hardware reservations with a price for the whole month, regardless of the
			// start and end filters. We pro-rate across the entire month.
			// Note that this will only work if startTime and endTime are actually a full day!
			if u.Type == "HardwareReservation" {
				if up.endTime.Sub(up.startTime).Seconds() != 86400 {
					// If this is not a full day, ignore this record
					continue
				}
				daysInMonth := float64(time.Date(up.startTime.Year(), up.startTime.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day())
				u.Price = u.Price / daysInMonth
				u.Total = u.Total / daysInMonth
				u.Name = "Hardware Reservation daily pro-rated"
			}

			bqU := usageRecord{
				startTime: up.startTime,
				endTime:   up.endTime,
				project:   project,
				metro:     u.Metro,
				plan:      u.Plan,
				tpe:       u.Type,
				name:      u.Name,
				price:     u.Price,
				quantity:  u.Quantity,
				total:     u.Total,
			}
			items = append(items, bqU)
		}

		log.Infof("%s: inserting %d records", project, len(items))

		if err = inserter.Put(ctx, items); err != nil {
			log.Error("Error while bulk-inserting items to BigQuery\n%s\n", err.Error())
			os.Exit(1)
		}
	}
}
