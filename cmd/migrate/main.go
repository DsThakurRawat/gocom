/*
What this file does
This is the database migration runner — a separate binary (not part of the main
API server) that manages your database schema changes over time. Think of it as
version control for your database structure.

Migrations solve a real problem: as your app evolves, you need to add tables,
modify columns, add indexes. Without migrations, you'd be running raw SQL by hand
and hoping every developer (and every environment — dev, staging, production) stays
in sync. Migrations make schema changes reproducible, ordered, and reversible.

This uses the golang-migrate library which reads SQL files from a directory
(cmd/migrate/migrations/) and applies them in order. Each migration has an UP
(apply the change) and a DOWN (reverse it). The library tracks which migrations
have been applied in a special schema_migrations table in your database.

Usage:
  go run cmd/migrate/main.go up    — apply all pending migrations
  go run cmd/migrate/main.go down  — roll back the last migration
Or via the Makefile:
  make migrate-up / make migrate-down
*/

package main

import (
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql" // blank import: registers the MySQL driver with database/sql
	mysqlDriver "github.com/go-sql-driver/mysql"           // named import: we use mysql.Config directly
	"github.com/golang-migrate/migrate/v4"                  // 3rd-party: the migration engine
	mysqlMigrate "github.com/golang-migrate/migrate/v4/database/mysql" // MySQL-specific migration driver
	_ "github.com/golang-migrate/migrate/v4/source/file"    // blank import: registers the "file://" source
	"github.com/DsThakurRawat/gocom/configs"
	"github.com/DsThakurRawat/gocom/db"
)

func main() {
	// Same DB config as the main server — reads from environment / .env file.
	cfg := mysqlDriver.Config{
		User:                 configs.Envs.DBUser,
		Passwd:               configs.Envs.DBPassword,
		Addr:                 configs.Envs.DBAddress,
		DBName:               configs.Envs.DBName,
		Net:                  "tcp",
		AllowNativePasswords: true,
		ParseTime:            true,
	}

	db, err := db.NewMySQLStorage(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// Create a migration driver instance wrapping our DB connection.
	// This lets the migrate library talk to MySQL specifically.
	driver, err := mysqlMigrate.WithInstance(db, &mysqlMigrate.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Point the migration engine at our SQL files directory.
	// "file://cmd/migrate/migrations" is a URL scheme the file source understands.
	// "mysql" is the database name for the migration tracking table.
	m, err := migrate.NewWithDatabaseInstance(
		"file://cmd/migrate/migrations",
		"mysql",
		driver,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Log the current migration version for visibility.
	// `dirty` means a migration was started but didn't complete cleanly.
	v, d, _ := m.Version()
	log.Printf("Version: %d, dirty: %v", v, d)

	// Read the last CLI argument to decide direction: "up" or "down".
	// os.Args is the raw command-line arguments slice.
	cmd := os.Args[len(os.Args)-1]
	if cmd == "up" {
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	}
	if cmd == "down" {
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatal(err)
		}
	}

}

/*
Things to remember from this file
Blank imports (_ "package"). These don't give you any symbols to use in code —
they exist purely for their init() side effects. The MySQL driver registers itself
with database/sql, and the file source registers itself with the migrate library.
Without these imports, the drivers silently don't exist and you get cryptic
"unknown driver" errors.

Named imports (mysqlDriver, mysqlMigrate). When two packages have the same last
segment (both called "mysql"), Go requires you to alias at least one. This is
just a naming conflict resolution, nothing fancy.

migrate.ErrNoChange is not an error. When there are no pending migrations, the
library returns this sentinel error. Checking for it (err != migrate.ErrNoChange)
prevents the program from crashing when everything is already up to date.

This is a separate binary. It has its own main() function — it's NOT part of
your API server. You run it independently (go run cmd/migrate/main.go up).
This is why it lives under cmd/migrate/ — Go convention puts each binary's
entry point in its own cmd/ subdirectory.
*/
