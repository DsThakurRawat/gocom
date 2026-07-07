/*
What this file doesThis is the product data access layer — the same Repository pattern as your user store, implementing the types.ProductStore interface. It's the "how" behind the product database operations: fetch one, fetch many, fetch all, create, update.Most of it mirrors the user store you already understand, so I'll comment briefly on the familiar parts and focus on the one genuinely new and interesting piece: GetProductsByID. That method solves a real problem — "give me products for a whole shopping cart at once" — using a dynamic IN (...) query. This is the technique that avoids the dreaded N+1 query problem (looping and hitting the DB once per cart item). Instead you fetch all cart products in a single query.*/

package product

import (
	"database/sql"
	"fmt"
	"strings" // used to dynamically build the "?,?,?" placeholder list

	"github.com/DsThakurRawat/gocom/types"
)

// Same Store + constructor + DI pattern as the user package.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetProductByID — single lookup. Identical shape to GetUserByID.
func (s *Store) GetProductByID(productID int) (*types.Product, error) {
	rows, err := s.db.Query("SELECT * FROM products WHERE id = ?", productID)
	if err != nil {
		return nil, err
	}
	p := new(types.Product)
	for rows.Next() {
		p, err = scanRowsIntoProduct(rows)
		if err != nil {
			return nil, err
		}
	}
	// (Note: unlike the user store, there's no "not found" check here — a
	// missing product returns a zero-valued Product with ID 0, not an error.)
	return p, nil
}

// GetProductsByID — the STAR of this file. Fetch MANY products in ONE query.
// Used at checkout: "load every product in the user's cart at once."
func (s *Store) GetProductsByID(productIDs []int) ([]types.Product, error) {
	// GOAL: build a query like  SELECT * FROM products WHERE id IN (?,?,?)
	// with exactly one ? per id, so the values stay PARAMETERIZED (injection-safe).

	// strings.Repeat(",?", n-1) makes the placeholders AFTER the first one.
	// For 3 ids: it produces ",?,?" — then the query template supplies the
	// leading "?" giving "(?,?,?)". (First "?" is hardcoded, rest are repeated.)
	placeholders := strings.Repeat(",?", len(productIDs)-1)
	query := fmt.Sprintf("SELECT * FROM products WHERE id IN (?%s)", placeholders)
	// IMPORTANT: fmt.Sprintf here only builds the STRUCTURE (the ?s), never the
	// actual values — so this is still safe from SQL injection. The ids go in
	// as parameters below, not concatenated into the string.

	// db.Query wants variadic ...interface{}, but we have a []int. Go won't let
	// you spread []int into ...interface{} directly, so convert element by element.
	args := make([]interface{}, len(productIDs))
	for i, v := range productIDs {
		args[i] = v
	}

	// args... "spreads" the slice into individual arguments matching each ?.
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}

	// Collect every matching row into a slice of VALUES (not pointers here).
	products := []types.Product{}
	for rows.Next() {
		p, err := scanRowsIntoProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, *p) // *p dereferences the pointer to store the value
	}
	return products, nil
}

// GetProducts — fetch the entire catalog. No WHERE clause.
func (s *Store) GetProducts() ([]*types.Product, error) {
	rows, err := s.db.Query("SELECT * FROM products")
	if err != nil {
		return nil, err
	}
	// make([]*T, 0) — an initialized empty slice. Preferred over nil so JSON
	// encodes it as [] (empty array) rather than null when there are no rows.
	products := make([]*types.Product, 0)
	for rows.Next() {
		p, err := scanRowsIntoProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, nil
}

// CreateProduct — INSERT. Note it takes the PAYLOAD type (client-settable
// fields only), not the full Product — no ID/CreatedAt, the DB sets those.
func (s *Store) CreateProduct(product types.CreateProductPayload) error {
	_, err := s.db.Exec(
		"INSERT INTO products (name, price, image, description, quantity) VALUES (?, ?, ?, ?, ?)",
		product.Name, product.Price, product.Image, product.Description, product.Quantity,
	)
	if err != nil {
		return err
	}
	return nil
}

// UpdateProduct — full UPDATE by id. The WHERE id = ? is what scopes it to one
// row; forget that clause and you'd overwrite EVERY product (a classic disaster).
func (s *Store) UpdateProduct(product types.Product) error {
	_, err := s.db.Exec(
		"UPDATE products SET name = ?, price = ?, image = ?, description = ?, quantity = ? WHERE id = ?",
		product.Name, product.Price, product.Image, product.Description, product.Quantity, product.ID,
	)
	if err != nil {
		return err
	}
	return nil
}

// scanRowsIntoProduct — row → struct mapping. ORDER MUST MATCH the table's
// column order (id, name, description, image, price, quantity, createdAt).
func scanRowsIntoProduct(rows *sql.Rows) (*types.Product, error) {
	product := new(types.Product)
	err := rows.Scan(
		&product.ID,
		&product.Name,
		&product.Description,
		&product.Image,
		&product.Price,
		&product.Quantity,
		&product.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return product, nil
}

/*
Things to remember from this file
Dynamic IN (...) queries — the safe way. When you need WHERE id IN (?, ?, ?) with a variable number of values, you build the placeholder string dynamically but keep the values as parameters. strings.Repeat generates the ?s; the ids go through args.... The injection-safety rule holds: Sprintf shapes structure, never data.
The N+1 problem. GetProductsByID exists so you can load a whole cart in one round-trip instead of looping and querying per item. Fetching N items in 1 query vs N queries is a huge performance difference at scale. Recognizing and avoiding N+1 is a core backend skill.
args... spreading + the []int → []interface{} conversion. db.Query takes variadic interface{}, and Go won't auto-convert a typed slice. That little conversion loop is boilerplate you'll write whenever you pass a dynamic list of args.
Pointer vs value slices. This file mixes []types.Product (values) and []*types.Product (pointers) across methods. Both work; pointers avoid copying the struct and let you represent "nil = missing," while values are simpler and safer to pass around. Just notice the inconsistency — real codebases usually pick one convention.
make([]T, 0) vs nil slice. Returning an initialized empty slice makes JSON serialize to [] instead of null, which front-end clients almost always prefer.
The WHERE clause is load-bearing. In UpdateProduct, WHERE id = ? is the only thing preventing a mass-overwrite of every row. Always double-check WHERE clauses on UPDATE and DELETE.
Now the same improvement notes that applied to the user store apply here too, plus one sharp edge unique to this file:
GetProductsByID crashes on an empty slice. If productIDs is empty, len(productIDs)-1 is -1, and strings.Repeat(",?", -1) panics ("negative Repeat count"). Before calling this, guard against an empty cart — e.g. if len(productIDs) == 0 { return []types.Product{}, nil }. This is a real bug worth fixing.
SELECT * + unclosed rows + no rows.Err() check — same three habits from the user store. Explicit column lists, defer rows.Close(), and checking rows.Err() after the loop are the production upgrades.
You now have the full product store. The matching product handler (HTTP routes for listing products, creating them — likely admin-only behind your JWT middleware) is the natural next file, and it'll look a lot like the user handler you already understand. Or jump to the cart/order logic, where GetProductsByID and that atomic-quantity concern from your types file finally come into play. Which way do you want to go?



*/
