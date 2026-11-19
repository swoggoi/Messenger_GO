// Command migrate applies SQL migrations from ./migrations using the same
// schema_migrations table as the golang-migrate CLI. It needs no extra tool
// installation: `go run ./cmd/migrate up`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/joho/godotenv"
)

type execer interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

var migrationName = regexp.MustCompile(`^([0-9]+)_(.+)\.(up|down)\.sql$`)
var legacyName = regexp.MustCompile(`^([0-9]+)_(.+)\.sql$`)

type migration struct {
	version int
	name    string
	up      string
	down    string
}

func loadMigrations(dir string) (map[int]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := map[int]migration{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if m := migrationName.FindStringSubmatch(entry.Name()); m != nil {
			version, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			item := result[version]
			item.version = version
			item.name = m[2]
			switch m[3] {
			case "up":
				item.up = entry.Name()
			case "down":
				item.down = entry.Name()
			}
			result[version] = item
			continue
		}
		// Backward compatibility with the original 001_create_users.sql name.
		if m := legacyName.FindStringSubmatch(entry.Name()); m != nil {
			version, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			item := result[version]
			if item.up == "" {
				item.version = version
				item.name = m[2]
				item.up = entry.Name()
				result[version] = item
			}
		}
	}
	return result, nil
}

func sortedVersions(migrations map[int]migration) []int {
	versions := make([]int, 0, len(migrations))
	for version := range migrations {
		versions = append(versions, version)
	}
	sort.Ints(versions)
	return versions
}

// splitStatements keeps dollar-quoted blocks (DO $$ ... $$) and comments intact.
func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	var quote rune
	var dollarTag string
	lineComment, blockComment := false, false
	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if lineComment {
			current.WriteRune(ch)
			if ch == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			current.WriteRune(ch)
			if ch == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				current.WriteRune('/')
				i++
				blockComment = false
			}
			continue
		}
		if dollarTag != "" {
			current.WriteRune(ch)
			if strings.HasPrefix(string(runes[i:]), dollarTag) {
				current.WriteString(dollarTag[1:])
				i += len(dollarTag) - 1
				dollarTag = ""
			}
			continue
		}
		if quote != 0 {
			current.WriteRune(ch)
			if ch == quote {
				if i+1 < len(runes) && runes[i+1] == quote {
					current.WriteRune(runes[i+1])
					i++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch {
		case ch == '-' && i+1 < len(runes) && runes[i+1] == '-':
			lineComment = true
			current.WriteRune(ch)
		case ch == '/' && i+1 < len(runes) && runes[i+1] == '*':
			blockComment = true
			current.WriteRune(ch)
		case ch == '\'' || ch == '"':
			quote = ch
			current.WriteRune(ch)
		case ch == '$':
			if tag := readDollarTag(runes[i:]); tag != "" {
				dollarTag = tag
				current.WriteString(tag)
				i += len(tag) - 1
			} else {
				current.WriteRune(ch)
			}
		case ch == ';':
			statements = append(statements, current.String())
			current.Reset()
		default:
			current.WriteRune(ch)
		}
	}
	if strings.TrimSpace(current.String()) != "" {
		statements = append(statements, current.String())
	}
	return statements
}

func readDollarTag(runes []rune) string {
	if len(runes) < 2 || runes[0] != '$' {
		return ""
	}
	end := 1
	for end < len(runes) && (runes[end] == '_' || runes[end] >= '0' && runes[end] <= '9' ||
		runes[end] >= 'a' && runes[end] <= 'z' || runes[end] >= 'A' && runes[end] <= 'Z') {
		end++
	}
	if end < len(runes) && runes[end] == '$' {
		return string(runes[:end+1])
	}
	return ""
}

func applyFile(ctx context.Context, conn execer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, statement := range splitStatements(string(data)) {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func currentVersion(ctx context.Context, conn *pgx.Conn) (int, bool, error) {
	var version int64
	var dirty bool
	err := conn.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&version, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return int(version), dirty, err
}

func setVersion(ctx context.Context, conn execer, version int, delete bool) error {
	if delete {
		_, err := conn.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version)
		return err
	}
	// golang-migrate хранит одну текущую версию; сохраняем тот же формат,
	// чтобы оба инструмента видели одинаковое состояние.
	if _, err := conn.Exec(ctx, `DELETE FROM schema_migrations`); err != nil {
		return err
	}
	_, err := conn.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)`, version)
	return err
}

func run(ctx context.Context, conn *pgx.Conn, dir, command string, count int) error {
	migrations, err := loadMigrations(dir)
	if err != nil {
		return err
	}
	versions := sortedVersions(migrations)
	if len(versions) == 0 {
		return errors.New("no migrations found")
	}
	for _, version := range versions {
		if migrations[version].up == "" || migrations[version].down == "" {
			return fmt.Errorf("migration %03d must have both .up.sql and .down.sql files", version)
		}
	}

	version, dirty, err := currentVersion(ctx, conn)
	if err != nil {
		return err
	}
	switch command {
	case "status":
		fmt.Printf("version: %d\ndirty: %t\napplied files: %d\n", version, dirty, countApplied(versions, version))
		return nil
	case "force":
		return setVersion(ctx, conn, count, false)
	case "up":
		if dirty {
			return fmt.Errorf("migration %d is dirty; fix it and run `force %d`", version, version)
		}
		limit := len(versions)
		if count > 0 {
			limit = count
		}
		applied := 0
		for _, next := range versions {
			if next <= version || applied >= limit {
				continue
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				return err
			}
			if err := applyFile(ctx, tx, filepath.Join(dir, migrations[next].up)); err != nil {
				_ = tx.Rollback(ctx)
				_, _ = conn.Exec(ctx, `DELETE FROM schema_migrations`)
				_, _ = conn.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, true)`, next)
				return err
			}
			if err := setVersion(ctx, tx, next, false); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			fmt.Printf("applied %03d_%s.up.sql\n", next, migrations[next].name)
			applied++
		}
		return nil
	case "down":
		if dirty {
			return fmt.Errorf("migration %d is dirty; fix it and run `force %d`", version, version)
		}
		if count <= 0 {
			count = 1
		}
		for i := 0; i < count && version > 0; i++ {
			item, ok := migrations[version]
			if !ok {
				return fmt.Errorf("migration %d not found", version)
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				return err
			}
			if err := applyFile(ctx, tx, filepath.Join(dir, item.down)); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if err := setVersion(ctx, tx, version, true); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			fmt.Printf("reverted %03d_%s.down.sql\n", version, item.name)
			version--
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func countApplied(versions []int, version int) int {
	count := 0
	for _, v := range versions {
		if v <= version {
			count++
		}
	}
	return count
}

func main() {
	_ = godotenv.Load()
	dir := flag.String("path", "migrations", "directory with SQL migrations")
	database := flag.String("database", os.Getenv("DB_SOURCE"), "PostgreSQL DSN (defaults to DB_SOURCE)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("usage: migrate [-path ./migrations] [-database DSN] <up|down|status|force> [N]")
		os.Exit(2)
	}
	command := args[0]
	count := 0
	if len(args) > 1 {
		parsed, err := strconv.Atoi(args[1])
		if err != nil {
			fmt.Println("N must be a number:", err)
			os.Exit(2)
		}
		count = parsed
	}
	if *database == "" {
		fmt.Println("DB_SOURCE is not set")
		os.Exit(1)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, *database)
	if err != nil {
		fmt.Println("postgres connection error:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY, dirty boolean NOT NULL)`); err != nil {
		fmt.Println("cannot initialise schema_migrations:", err)
		os.Exit(1)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(748291337)`); err != nil {
		fmt.Println("cannot lock migrations:", err)
		os.Exit(1)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(748291337)`) }()

	if err := run(ctx, conn, *dir, command, count); err != nil {
		fmt.Println("migration error:", err)
		os.Exit(1)
	}
}
