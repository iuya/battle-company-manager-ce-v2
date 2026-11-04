package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"

	"go-simpler.org/env"

	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// runArchive archives ended campaigns that have no archive: the ones ended before archives existed, and any whose archiving failed when
// they were ended (docs/archive.md, "Campaigns ended before archives"). It is final for each campaign it archives, so it asks first.
//
//	server archive --campaign 12          one campaign
//	server archive --campaign all-finished  every ended campaign that has no archive
//	server archive --campaign all-finished --yes   without asking
func runArchive(args []string) error {
	fs := flag.NewFlagSet("archive", flag.ContinueOnError)
	which := fs.String("campaign", "", "a campaign id, or all-finished")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *which == "" {
		return fmt.Errorf("usage: server archive --campaign <id|all-finished> [--yes]")
	}
	var cfg struct {
		DatabaseURL string `env:"DATABASE_URL,required"`
	}
	if err := env.Load(&cfg, nil); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	b, err := newBackend(ctx, store)
	if err != nil {
		return err
	}
	pending, err := store.Archives().Unarchived(ctx)
	if err != nil {
		return err
	}
	var ids []int64
	if *which == "all-finished" {
		ids = pending
	} else {
		id, err := strconv.ParseInt(*which, 10, 64)
		if err != nil {
			return fmt.Errorf("--campaign wants a campaign id or all-finished, not %q", *which)
		}
		if !slices.Contains(pending, id) {
			return fmt.Errorf("campaign %d is not an ended campaign without an archive", id)
		}
		ids = []int64{id}
	}
	if len(ids) == 0 {
		fmt.Println("Nothing to archive: every ended campaign has its archive.")
		return nil
	}
	fmt.Printf("%d ended campaign(s) will become read-only archives, ids %v.\nTheir rows (units, matches, live data, grants) are replaced by one document and cannot be restored.\n", len(ids), ids)
	if !*yes {
		fmt.Print("Type yes to go on: ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "yes" {
			return fmt.Errorf("not confirmed; nothing was archived")
		}
	}
	failed := 0
	for _, id := range ids {
		if err := b.archiver.Archive(ctx, id); err != nil {
			failed++
			fmt.Printf("campaign %d: FAILED: %v\n", id, err)
			continue
		}
		fmt.Printf("campaign %d: archived\n", id)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d campaigns could not be archived", failed, len(ids))
	}
	return nil
}
