// Command seed manages the dataset (SPEC F4):
//
//	seed build    [-seed N] [-schema v0]   create the template database
//	seed reset    [-strategy FILE_COPY]    replace the run database with a fresh clone
//	seed checksum [-db stresslab_seed]     hash every table, to prove two builds match
//
// The admin connection comes from -admin-url or STRESSLAB_ADMIN_URL.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/adssib/StressLab/internal/seed"
	"github.com/adssib/StressLab/internal/telemetry"
)

const defaultAdminURL = "postgres://postgres:postgres@localhost:5433/postgres"

func main() {
	log := telemetry.NewLogger(os.Stderr)
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	adminURL := fs.String("admin-url", envOr("STRESSLAB_ADMIN_URL", defaultAdminURL), "Postgres URL of the admin database")
	seedValue := fs.Uint64("seed", 1, "dataset seed (build)")
	schema := fs.String("schema", "v0", "schema version (build)")
	strategy := fs.String("strategy", string(seed.FileCopy), "clone strategy: FILE_COPY or WAL_LOG (reset)")
	db := fs.String("db", seed.TemplateDB, "database to hash (checksum)")
	_ = fs.Parse(args)

	ctx := context.Background()
	var err error
	switch cmd {
	case "build":
		start := time.Now()
		err = seed.Build(ctx, *adminURL, *schema, *seedValue, log)
		if err == nil {
			fmt.Printf("built %s (seed %d, schema %s) in %s\n",
				seed.TemplateDB, *seedValue, *schema, time.Since(start).Round(time.Millisecond))
		}
	case "reset":
		var took time.Duration
		took, err = seed.Reset(ctx, *adminURL, seed.Strategy(*strategy))
		if err == nil {
			fmt.Printf("reset %s from %s with %s in %s\n",
				seed.RunDB, seed.TemplateDB, *strategy, took.Round(time.Millisecond))
		}
	case "checksum":
		var sums map[string]string
		sums, err = seed.Checksum(ctx, *adminURL, *db)
		if err == nil {
			tables := make([]string, 0, len(sums))
			for t := range sums {
				tables = append(tables, t)
			}
			sort.Strings(tables)
			for _, t := range tables {
				fmt.Printf("%-6s %s\n", t, sums[t])
			}
		}
	default:
		usage()
	}
	if err != nil {
		log.Error(cmd+" failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: seed build|reset|checksum [flags]")
	os.Exit(2)
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
