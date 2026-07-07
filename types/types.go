package types

// The "types" package centralizes all data models and interfaces for the app.
// Keeping models in one place is a common Go pattern: your database layer,
// HTTP handlers, and business logic all import from here, so there's a single
// source of truth for what your data looks like.

import (
	"time" // time.Time is Go's built-in type for timestamps (dates + times)
)

// ============================================================
// DOMAIN MODELS
// These structs mirror the actual rows/records in your database.
// The `json:"..."` struct tags control how each field is named
// when marshaled to JSON (what your API sends) or unmarshaled
// from JSON (what your API receives).
// ============================================================

type User struct {
	ID        int       `json:"id"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`         // `-` = NEVER expose in JSON. Critical security practice:
	                                         // even the hashed password must never leak in API responses.
	CreatedAt time.Time `json:"createdAt"`  // audit field: when the record was created
}

type Product struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Image       string  `json:"image"`
	Price       float64 `json:"price"` // money as float64 is fine for learning, but in real
	                                    // production systems prefer integer cents or a decimal
	                                    // type — floats cause rounding errors with currency.
	// note that this isn't the best way to handle quantity
	// because it's not atomic (in ACID), but it's good enough for this example.
	// "Atomic" = the read-check-update of stock should happen as one indivisible DB
	// operation, otherwise two simultaneous orders could both see stock=1 and both
	// succeed (a race condition / overselling). Real fix: DB transactions + row locks.
	Quantity  int       `json:"quantity"`
	CreatedAt time.Time `json:"createdAt"`
}

// CartCheckoutItem represents a single line the client sends at checkout:
// "I want this many of this product." The server looks up the real price
// itself — you never trust a price sent by the client.
type CartCheckoutItem struct {
	ProductID int `json:"productID"`
	Quantity  int `json:"quantity"`
}

// Order is the "header" of a purchase — one per checkout.
type Order struct {
	ID        int       `json:"id"`
	UserID    int       `json:"userID"` // foreign key → which User placed this order
	Total     float64   `json:"total"`
	Status    string    `json:"status"`  // e.g. "pending", "paid", "shipped" — a state machine
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrderItem is a single product line within an Order (one-to-many: an Order has many OrderItems).
// Price is copied in here on purpose: it's a historical snapshot of what the product
// cost AT THE TIME OF PURCHASE, so later price changes don't rewrite past orders.
type OrderItem struct {
	ID        int       `json:"id"`
	OrderID   int       `json:"orderID"`   // foreign key → parent Order
	ProductID int       `json:"productID"` // foreign key → which Product
	Quantity  int       `json:"quantity"`
	Price     float64   `json:"price"`
	CreatedAt time.Time `json:"createdAt"`
}

// ============================================================
// STORE INTERFACES  (the "Repository" pattern)
// An interface defines WHAT operations exist without saying HOW.
// Your concrete DB code (e.g. a MySQL/Postgres struct) implements these.
// Why bother?
//   1. Decoupling — handlers depend on the interface, not on MySQL directly.
//   2. Testability — in tests you swap in a fake/mock store, no real DB needed.
// This is "dependency inversion": high-level code depends on abstractions.
// ============================================================

type UserStore interface {
	// Returning *User (a pointer) + error is idiomatic Go: nil pointer on "not found",
	// and the error carries the reason things failed.
	GetUserByEmail(email string) (*User, error)
	GetUserByID(id int) (*User, error)
	CreateUser(User) error
}

type ProductStore interface {
	GetProductByID(id int) (*Product, error)
	GetProductsByID(ids []int) ([]Product, error) // batch fetch — avoids N+1 queries in the cart
	GetProducts() ([]*Product, error)
	CreateProduct(CreateProductPayload) error
	UpdateProduct(Product) error
}

type OrderStore interface {
	CreateOrder(Order) (int, error) // returns the new order's ID so items can reference it
	CreateOrderItem(OrderItem) error
}

// ============================================================
// PAYLOAD STRUCTS  (Data Transfer Objects / DTOs)
// These are the shapes of INCOMING request bodies — separate from the
// domain models above. Two big reasons to keep them separate:
//   1. Security — a payload only exposes fields the client is ALLOWED to set.
//      (Notice you never let the client send an `ID` or `CreatedAt`.)
//   2. Validation — the `validate:"..."` tags are read by a validation
//      library (go-playground/validator) to enforce rules before you
//      trust the data. Golden rule of backend dev: never trust client input.
// ============================================================

type CreateProductPayload struct {
	Name        string  `json:"name" validate:"required"`  // must be present & non-empty
	Description string  `json:"description"`               // optional (no rule)
	Image       string  `json:"image"`
	Price       float64 `json:"price" validate:"required"`
	Quantity    int     `json:"quantity" validate:"required"`
}

type RegisterUserPayload struct {
	FirstName string `json:"firstName" validate:"required"`
	LastName  string `json:"lastName" validate:"required"`
	Email     string `json:"email" validate:"required,email"`             // must be a valid email format
	Password  string `json:"password" validate:"required,min=3,max=130"`  // length bounds.
	                                                                    // (min=3 is low for real apps — 8+ is safer.)
	                                                                    // This is the PLAINTEXT password from the client;
	                                                                    // you hash it (e.g. bcrypt) before storing.
}

type LoginUserPayload struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// CartCheckoutPayload wraps a slice of items. `validate:"required"` on a slice
// ensures the client actually sent items (not an empty/nil cart).
type CartCheckoutPayload struct {
	Items []CartCheckoutItem `json:"items" validate:"required"`
}