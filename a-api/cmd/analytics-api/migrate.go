package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/metap-org/waap/a-api/store"
)

const migrateUsage = `Usage: analytics-api migrate <command>

  status                      what is pending, and the table's retention (reads only)
  up [--dry-run]              apply the pending migrations, or print their SQL
  down [--steps N] [--yes]    undo the newest N; without --yes the SQL is only printed
  retention [--yes]           set the table's TTL to ANALYTICS_RETENTION_DAYS; without --yes
                              the SQL is only printed. A shorter one deletes older events.

CLICKHOUSE_URL, ANALYTICS_DATABASE and ANALYTICS_RETENTION_DAYS are read as by the service.`

var errNotConfirmed = errors.New("nothing was run: repeat with --yes to run the SQL above")

func migrateCommand(ctx context.Context, st *store.ClickHouse, retention int, args []string) error {
	if len(args) == 0 {
		fmt.Println(migrateUsage)
		return errors.New("a migrate command is required")
	}
	flags := flag.NewFlagSet("migrate "+args[0], flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "")
	yes := flags.Bool("yes", false, "")
	steps := flags.Int("steps", 1, "")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	show := func(statements []string) { fmt.Println(strings.Join(statements, ";\n\n") + ";") }
	switch args[0] {
	case "status":
		if err := st.CheckSchema(ctx, retention); err != nil {
			return err
		}
		fmt.Println("up to date")
	case "up":
		if *dryRun {
			pending, err := st.Pending(ctx, retention)
			if err != nil {
				return err
			}
			for _, m := range pending {
				fmt.Printf("-- ===== %04d_%s.up.sql =====\n", m.Version, m.Name)
				show(m.Up)
			}
			return nil
		}
		done, err := st.Up(ctx, retention)
		if err != nil {
			return err
		}
		fmt.Println("applied:", done)
	case "down":
		targets, err := st.LastApplied(ctx, retention, *steps)
		if err != nil {
			return err
		}
		for _, m := range targets {
			fmt.Printf("-- ===== %04d_%s.down.sql =====\n", m.Version, m.Name)
			show(m.Down)
		}
		if !*yes {
			return errNotConfirmed
		}
		return st.Down(ctx, targets)
	case "retention":
		have, err := st.RetentionDays(ctx)
		if err != nil {
			return err
		}
		if have == 0 {
			return errors.New("there is no table yet: run migrate up")
		}
		fmt.Printf("-- the table keeps %d days, ANALYTICS_RETENTION_DAYS is %d\n", have, retention)
		show([]string{st.RetentionSQL(retention)})
		if !*yes {
			return errNotConfirmed
		}
		return st.SetRetention(ctx, retention)
	default:
		fmt.Println(migrateUsage)
		return fmt.Errorf("unknown migrate command %q", args[0])
	}
	return nil
}
