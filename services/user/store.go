/*

What this file does
This is your data access layer for users — the concrete implementation of that UserStore interface you defined earlier in types. Remember how the interface said what operations exist (GetUserByEmail, GetUserByID, CreateUser) but not how? This file is the "how." It's the code that actually talks to the SQL database.
This is the Repository pattern in action. Your handlers and middleware depend on the types.UserStore interface; this Store struct fulfills that contract using MySQL. The benefit: if you ever swapped MySQL for Postgres, or wrote a fake store for testing, only this file changes — nothing else in your app cares, because everything else talks to the interface.
The whole file revolves around one thing: translating between SQL rows and Go structs. Writing a user in means turning struct fields into an INSERT statement. Reading a user out means running a SELECT, then copying each column of the result row into the fields of a types.User. That copying step is what scanRowsIntoUser handles.






*/















package user

import (
	"database/sql" // stdlib: Go's generic SQL interface (works with MySQL, Postgres, etc.)
	"fmt"

	"github.com/DsThakurRawat/gocom/types"
)

// Store holds a handle to the database connection pool.
// It's a struct with ONE job: run user-related SQL. The methods on it
// (below) are what satisfy the types.UserStore interface.
type Store struct {
	db *sql.DB // *sql.DB is a POOL of connections, safe for concurrent use — not a single connection.
}

// NewStore is a CONSTRUCTOR. Go has no built-in constructors, so the
// convention is a NewXxx function that builds and returns the struct.
// You inject the db from outside (dependency injection) rather than
// creating it inside — keeps this package decoupled from connection setup.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateUser inserts a new user row.
func (s *Store) CreateUser(user types.User) error {
	// s.db.Exec runs a statement that returns NO rows (INSERT/UPDATE/DELETE).
	//
	// CRITICAL SECURITY POINT: the `?` are PARAMETERIZED PLACEHOLDERS.
	// The values are sent to the DB separately from the query text, so a
	// malicious email like "'; DROP TABLE users;--" is treated as plain
	// data, NOT executable SQL. This is how you prevent SQL INJECTION.
	// NEVER build queries with string concatenation / fmt.Sprintf.
	_, err := s.db.Exec(
		"INSERT INTO users (firstName, lastName, email, password) VALUES (?, ?, ?, ?)",
		user.FirstName, user.LastName, user.Email, user.Password,
	)
	// We ignore the first return value (sql.Result) with `_` since we don't
	// need the inserted ID here.
	if err != nil {
		return err
	}
	return nil
}

// GetUserByEmail fetches one user by email (used during login).
func (s *Store) GetUserByEmail(email string) (*types.User, error) {
	// Query runs a SELECT and returns multiple rows. Again, ? keeps it injection-safe.
	rows, err := s.db.Query("SELECT * FROM users WHERE email = ?", email)
	if err != nil {
		return nil, err
	}

	// new(types.User) allocates a zero-valued User and returns a pointer to it.
	u := new(types.User)
	// Iterate the result set. rows.Next() advances to each row; here we expect
	// at most one match, so the loop just ends up holding the last (only) row.
	for rows.Next() {
		u, err = scanRowsIntoUser(rows)
		if err != nil {
			return nil, err
		}
	}

	// If no row matched, u is still the zero value, so its ID is 0.
	// That's the signal for "not found". (IDs in the DB start at 1.)
	if u.ID == 0 {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

// GetUserByID does the same lookup but by primary key (used by the auth middleware).
func (s *Store) GetUserByID(id int) (*types.User, error) {
	rows, err := s.db.Query("SELECT * FROM users WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	u := new(types.User)
	for rows.Next() {
		u, err = scanRowsIntoUser(rows)
		if err != nil {
			return nil, err
		}
	}
	if u.ID == 0 {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

// scanRowsIntoUser copies the columns of the CURRENT row into a User struct.
// This is the manual "row → struct" mapping. Go's database/sql doesn't do it
// automatically, so you list each destination field explicitly.
func scanRowsIntoUser(rows *sql.Rows) (*types.User, error) {
	user := new(types.User)
	// rows.Scan takes POINTERS (&) to each field so it can WRITE the column
	// values into them. ORDER IS EVERYTHING: these must match the column
	// order returned by SELECT * — which is the order columns exist in the
	// table. This is exactly why SELECT * is fragile (see notes below).
	err := rows.Scan(
		&user.ID,
		&user.FirstName,
		&user.LastName,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}
/*


Things to remember from this file
Parameterized queries prevent SQL injection. This is the single most important security habit in database code. The ? placeholders + separate arguments mean user input can never be executed as SQL. The moment you're tempted to write fmt.Sprintf("... WHERE email = '%s'", email), stop — that's the classic injection hole.
The Repository pattern. This concrete Store implements the abstract types.UserStore interface. Interface = contract, Store = fulfillment. This separation is what makes your app testable and swappable.
Constructor + dependency injection. NewStore(db) takes the connection from outside instead of creating it. The database setup lives elsewhere (probably main.go), and gets passed in. This keeps concerns separated.
Scan needs pointers, in column order. rows.Scan(&user.ID, ...) writes into your struct, so it needs addresses. And the argument order must exactly match the SELECT's column order.
Now a few things worth improving as you level up — these are real weaknesses in this beginner-friendly code, not nitpicks:
SELECT * is fragile. Because Scan depends on exact column order, SELECT * breaks the moment someone reorders columns or adds a new one to the table — your Scan will silently map values into the wrong fields or error out. Production code lists columns explicitly: SELECT id, firstName, lastName, email, password, createdAt FROM users. This guarantees the order matches your Scan no matter what happens to the table.
Prefer QueryRow for single-row lookups. Fetching one user with Query + a for rows.Next() loop is clunky and, importantly, leaks the rows connection if you forget rows.Close() — which this code does forget. For a by-ID or by-email lookup, s.db.QueryRow(...).Scan(...) is cleaner, closes automatically, and returns a tidy sql.ErrNoRows for "not found" instead of the u.ID == 0 trick.
The u.ID == 0 "not found" check is a workaround. It works only because IDs start at 1. QueryRow with sql.ErrNoRows is the idiomatic way to express "no such user."
None of these break your app today — they're the difference between "works in a tutorial" and "safe in production." The injection safety, though, is non-negotiable and this code already gets that part right.
Want to continue to the user service/handler next? That's where these store methods get wired to HTTP routes, password hashing happens, and CreateJWT from the auth file finally gets called — the payoff where everything connects.





*/