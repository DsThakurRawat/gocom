/*
What this file does
This is the entry point of your entire application — the "main" function that Go
looks for to start execution. Its job is simple but critical: set up the database
connection, verify it works, and boot the HTTP server.

Here's the flow: it reads config from your environment (via configs.Envs), builds
a MySQL connection string (DSN), opens the connection pool, pings the DB to confirm
it's alive, and then hands everything off to the API server which starts listening
for HTTP requests.

The separation is clean: this file only does WIRING — it connects the pieces
(config, database, server) but contains zero business logic. All the real work
happens in the packages it imports. This is the "composition root" pattern: the
one place where you assemble your app's dependency graph.
*/

package main

import (
	"database/sql" // stdlib: Go's generic database interface
	"fmt"
	"log"

	"github.com/go-sql-driver/mysql"          // 3rd-party: MySQL driver (registers itself with database/sql)
	"github.com/DsThakurRawat/gocom/cmd/api"  // our API server package
	"github.com/DsThakurRawat/gocom/configs"  // our config loader (reads .env / environment)
	"github.com/DsThakurRawat/gocom/db"       // our DB setup package
)

func main() {
	// Build the MySQL connection config from environment variables.
	// mysql.Config is a struct from the go-sql-driver library that knows
	// how to format itself into a DSN string (user:pass@tcp(host:port)/dbname).
	// AllowNativePasswords and ParseTime are MySQL-specific settings:
	//   - AllowNativePasswords: allows older MySQL auth methods
	//   - ParseTime: converts MySQL DATETIME/TIMESTAMP columns to Go time.Time
	//     automatically (without this, you'd get raw strings)
		cfg := mysql.Config{
		User:                 configs.Envs.DBUser,
		Passwd:               configs.Envs.DBPassword,
		Addr:                 configs.Envs.DBAddress,
		DBName:               configs.Envs.DBName,
		Net:                  "tcp",
		AllowNativePasswords: true,
		ParseTime:            true,
	}

	// Open a connection pool to MySQL. Note: this doesn't actually connect yet!
	// sql.Open is lazy — it just validates the DSN and prepares the pool.
	// The actual TCP connection happens on first use (or when we Ping below).
	db, err := db.NewMySQLStorage(cfg)
	if err != nil {
		log.Fatal(err)
	}

	// NOW we actually talk to the database. Ping sends a real query to verify
	// the connection is alive. If the DB is down, we crash here — fail fast.
	initStorage(db)

	// Build the API server with the address (":8080") and the DB pool,
	// then start listening. Run() blocks forever (until the process is killed).
	server := api.NewAPIServer(fmt.Sprintf(":%s", configs.Envs.Port), db)
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}

// initStorage pings the database to verify the connection is alive.
// Called once at startup — if the DB is unreachable, we want to know
// immediately (fail fast) rather than discovering it on the first user request.
func initStorage(db *sql.DB) {
	err := db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("DB: Successfully connected!")
}

/*
Things to remember from this file
The composition root pattern. main() is where you wire everything together —
config, database, server. It imports packages but delegates all real work to them.
This keeps main small and makes each component independently testable.

sql.Open is lazy. It doesn't actually connect to the database — it just prepares
the connection pool. The real connection happens on Ping() or the first query.
That's why initStorage exists: to force the connection and fail fast if the DB
is down.

log.Fatal vs log.Println. Fatal calls os.Exit(1) after logging — the app dies.
Use it only for truly unrecoverable errors (can't connect to DB at startup).
For errors during normal operation, you'd return errors up the call chain instead.

The *sql.DB is a POOL, not a connection. It's safe for concurrent use across
goroutines. You create ONE pool at startup and pass it everywhere — never create
multiple pools.
*/
