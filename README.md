# equinix-billing-tools

This is a tool to get data from the Equinix Metal usages API.

## Building

`go build .`

## Prerequisites

You will need an Equinix Metal API token that has access to [the `usages` API](https://developer.equinix.com/catalog/metalv1#operation/findProjectUsage).
You will need to set the `EQUINIX_TOKEN` environment variable to this token.

## Usage

`equinix-billing-tools <subcommand> [<options>]`

### `cost_summary` subcommand

Displays a summary of costs per project for the given period and for a baseline
period of the same length. Due to quirks of the Equinix API, the tool generates
separate reports for Reserved Hardware and for everything else.

You can provide any or all of the following flags:

`-e`: This is the end date for the report, in `YYYY-MM-DD` format. The Equinix
Usages API has eventual consistency, experience has shown that it eventually
settles down sometime between 1 and 2 days after the usage. Therefore, the default
value is 2 days before the current date.

`-d`: This is the number of days to aggregate the report over. The default is one
day.

`-b`: This is the end date for the report baseline, in `YYYY-MM-DD` format. The
default value is the date before the report start date.

`-t`: This is the report type. Currently the tool supports two report types: reservations
and everything but reservations. The reason for this is that the Usages API will
always report the full month's usage regardless of the query period, so reservations
are separated out from the main report in order to prevent confusion.

The tool will print the report to stdout. The report has four columns:

* The project name
* The aggregated cost for the given period
* The aggregated cost for the baseline period
* The percentage change from the baseline to the given period

For example: if you run with the flags `-e 2023-01-22 -d 7` you will get the following:

* Report period: 2023-01-16 to 2023-01-22 (both inclusive)
* Baseline period: 2023-01-07 to 2023-01-15 (both inclusive)

### `bigquery` subcommand
