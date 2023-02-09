package main

import (
	"fmt"
	"os"

	"github.com/ipfs-shipyard/equinix-billing-tools/cmd"
	"github.com/ipfs-shipyard/equinix-billing-tools/equinix"
)

func main() {
	commands := map[string]func(equinix.Equinix) cmd.Command{
		"cost_summary": cmd.CostSummary,
		"bigquery":     cmd.UploadToBigquery,
	}

	equinixToken := os.Getenv("EQUINIX_TOKEN")

	if len(equinixToken) == 0 {
		fmt.Fprintf(os.Stderr, "EQUINIX_TOKEN environment variable is not set")
		os.Exit(1)
	}

	eq := equinix.Equinix{
		Token: equinixToken,
	}

	if len(os.Args) == 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s <subcommand> [<options>]\nValid subcommands: \n", os.Args[0])
		printSubcommands(commands)
		os.Exit(1)
	}

	command, found := commands[os.Args[1]]

	if !found {
		fmt.Fprintf(os.Stderr, "Invalid subcommand %s. Valid subcommands: \n", os.Args[1])
		printSubcommands(commands)
		os.Exit(1)
	}

	command(eq).Run()
}

func printSubcommands(commands map[string]func(equinix.Equinix) cmd.Command) {
	for k := range commands {
		fmt.Fprintf(os.Stderr, "    %s\n", k)
	}
}
