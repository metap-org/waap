// The schema is changed by a person running `analytics-api migrate`; a booting service only
// checks it. Statements are the files of ../migrations. This is a small runner of its own: this
// module may use only metap's pkg/kit.

package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/metap-org/waap/a-api/migrations"
)

const statementBreak = "-- migrate:statement-break"

// ErrSchemaNotReady: the database lacks something this build needs; nothing was changed.
var ErrSchemaNotReady = errors.New("the ClickHouse schema is not ready for this build")

// Migration is one numbered change, its statements with the placeholders filled in.
type Migration struct {
	Version  int
	Name     string
	Up, Down []string
}

var migrationFile = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

func (c *ClickHouse) database() string { return strings.TrimSuffix(c.table, ".security_events") }

func (c *ClickHouse) ledger() string { return c.database() + ".schema_migrations" }

// statements splits a file, fills the placeholders and drops the parts with only comments.
func (c *ClickHouse) statements(script string, retentionDays int) []string {
	script = strings.NewReplacer("{{database}}", c.database(), "{{retention_days}}", strconv.Itoa(retentionDays)).Replace(script)
	var out []string
	for _, part := range strings.Split(script, statementBreak) {
		var code []string
		for _, line := range strings.Split(part, "\n") {
			if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
				code = append(code, line)
			}
		}
		if len(code) > 0 {
			out = append(out, strings.Join(code, "\n"))
		}
	}
	return out
}

// Migrations are this build's migrations in order. Every one has both scripts.
func (c *ClickHouse) Migrations(retentionDays int) ([]Migration, error) {
	if retentionDays <= 0 {
		return nil, fmt.Errorf("retention must be at least 1 day, got %d", retentionDays)
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, err
	}
	byVersion := map[int]*Migration{}
	for _, e := range entries {
		m := migrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("%s: a migration file is NNNN_name.up.sql or .down.sql", e.Name())
		}
		version, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, err
		}
		script, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			return nil, err
		}
		entry := byVersion[version]
		if entry == nil {
			entry = &Migration{Version: version, Name: m[2]}
			byVersion[version] = entry
		} else if entry.Name != m[2] {
			return nil, fmt.Errorf("two migrations numbered %d: %s and %s", version, entry.Name, m[2])
		}
		if m[3] == "up" {
			entry.Up = c.statements(string(script), retentionDays)
		} else {
			entry.Down = c.statements(string(script), retentionDays)
		}
	}
	out := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		if len(m.Up) == 0 || len(m.Down) == 0 {
			return nil, fmt.Errorf("migration %d needs both an up and a down script", m.Version)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func (c *ClickHouse) tableDDL(ctx context.Context, name string) (string, error) {
	rows, err := c.conn.Query(ctx, "SELECT create_table_query FROM system.tables WHERE database = ? AND name = ?", c.database(), name)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ddl string
	if rows.Next() {
		if err := rows.Scan(&ddl); err != nil {
			return "", err
		}
	}
	return ddl, rows.Err()
}

// applied is the versions recorded in the ledger; none when there is no ledger.
func (c *ClickHouse) applied(ctx context.Context) (map[int]bool, error) {
	out := map[int]bool{}
	ddl, err := c.tableDDL(ctx, "schema_migrations")
	if err != nil || ddl == "" {
		return out, err
	}
	rows, err := c.conn.Query(ctx, "SELECT version FROM "+c.ledger())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v uint32
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[int(v)] = true
	}
	return out, rows.Err()
}

// Pending are the migrations not applied yet. It only reads.
func (c *ClickHouse) Pending(ctx context.Context, retentionDays int) ([]Migration, error) {
	all, err := c.Migrations(retentionDays)
	if err != nil {
		return nil, err
	}
	done, err := c.applied(ctx)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, m := range all {
		if !done[m.Version] {
			out = append(out, m)
		}
	}
	return out, nil
}

var ttlDays = regexp.MustCompile(`TTL .*?(?:toIntervalDay\((\d+)\)|INTERVAL (\d+) DAY)`)

// RetentionDays is the table's TTL in days; 0 when there is no table.
func (c *ClickHouse) RetentionDays(ctx context.Context) (int, error) {
	ddl, err := c.tableDDL(ctx, "security_events")
	if err != nil || ddl == "" {
		return 0, err
	}
	m := ttlDays.FindStringSubmatch(ddl)
	if m == nil {
		return 0, fmt.Errorf("%s has no day TTL", c.table)
	}
	return strconv.Atoi(m[1] + m[2])
}

// RetentionSQL sets the table's TTL. Shortening it makes ClickHouse delete the events that are
// now too old.
func (c *ClickHouse) RetentionSQL(retentionDays int) string {
	return fmt.Sprintf("ALTER TABLE %s MODIFY TTL toDateTime(occurred_at) + INTERVAL %d DAY", c.table, retentionDays)
}

// SetRetention runs RetentionSQL.
func (c *ClickHouse) SetRetention(ctx context.Context, retentionDays int) error {
	if retentionDays <= 0 {
		return fmt.Errorf("retention must be at least 1 day, got %d", retentionDays)
	}
	return c.conn.Exec(ctx, c.RetentionSQL(retentionDays))
}

// CheckSchema is the boot check: no migration pending and the TTL as configured. It changes nothing.
func (c *ClickHouse) CheckSchema(ctx context.Context, retentionDays int) error {
	pending, err := c.Pending(ctx, retentionDays)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%w: %d migration(s) not applied, the first is %04d_%s: run `analytics-api migrate up`",
			ErrSchemaNotReady, len(pending), pending[0].Version, pending[0].Name)
	}
	have, err := c.RetentionDays(ctx)
	if err != nil {
		return err
	}
	if have == 0 {
		return fmt.Errorf("%w: table %s does not exist although its migration is recorded", ErrSchemaNotReady, c.table)
	}
	if have != retentionDays {
		return fmt.Errorf("%w: the table keeps events %d days and ANALYTICS_RETENTION_DAYS is %d: run `analytics-api migrate retention`",
			ErrSchemaNotReady, have, retentionDays)
	}
	return nil
}

// Up applies the pending migrations in order, recording each after its last statement. ClickHouse
// has no transaction around them, so statements tolerate running again (IF NOT EXISTS). Run one
// at a time: nothing stops two concurrent runs.
func (c *ClickHouse) Up(ctx context.Context, retentionDays int) ([]int, error) {
	pending, err := c.Pending(ctx, retentionDays)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	for _, s := range []string{
		"CREATE DATABASE IF NOT EXISTS " + c.database(),
		"CREATE TABLE IF NOT EXISTS " + c.ledger() + " (version UInt32, name String, installed_on DateTime DEFAULT now()) ENGINE = MergeTree ORDER BY version",
	} {
		if err := c.conn.Exec(ctx, s); err != nil {
			return nil, err
		}
	}
	var done []int
	for _, m := range pending {
		for i, s := range m.Up {
			if err := c.conn.Exec(ctx, s); err != nil {
				return done, fmt.Errorf("migration %04d_%s, statement %d: %w", m.Version, m.Name, i+1, err)
			}
		}
		if err := c.conn.Exec(ctx, "INSERT INTO "+c.ledger()+" (version, name) VALUES (?, ?)", uint32(m.Version), m.Name); err != nil {
			return done, err
		}
		done = append(done, m.Version)
	}
	return done, nil
}

// LastApplied are the newest steps applied migrations, newest first.
func (c *ClickHouse) LastApplied(ctx context.Context, retentionDays, steps int) ([]Migration, error) {
	if steps < 1 {
		return nil, fmt.Errorf("rollback needs at least 1 step, got %d", steps)
	}
	all, err := c.Migrations(retentionDays)
	if err != nil {
		return nil, err
	}
	done, err := c.applied(ctx)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for i := len(all) - 1; i >= 0 && len(out) < steps; i-- {
		if done[all[i].Version] {
			out = append(out, all[i])
			delete(done, all[i].Version)
		} else if len(done) == 0 {
			break
		}
	}
	for v := range done {
		if len(out) < steps && v > all[len(all)-1].Version {
			return nil, fmt.Errorf("version %d is applied but this build does not have it: roll back with the build that applied it", v)
		}
	}
	return out, nil
}

// Down runs the down script of each of targets (from LastApplied) and forgets it.
func (c *ClickHouse) Down(ctx context.Context, targets []Migration) error {
	for _, m := range targets {
		for i, s := range m.Down {
			if err := c.conn.Exec(ctx, s); err != nil {
				return fmt.Errorf("rolling back %04d_%s, statement %d: %w", m.Version, m.Name, i+1, err)
			}
		}
		if err := c.conn.Exec(ctx, "ALTER TABLE "+c.ledger()+" DELETE WHERE version = ? SETTINGS mutations_sync = 1", uint32(m.Version)); err != nil {
			return err
		}
	}
	return nil
}
