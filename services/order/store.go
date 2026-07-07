/*
What this file does
This is the order data access layer — the Repository pattern implementation for
orders and order items. It's simpler than the user or product stores because
orders are (in this version) write-only: you create them at checkout but never
query them back.

Two operations live here: CreateOrder (inserts the order "header" — who bought,
total, status) and CreateOrderItem (inserts each line item — which product, how
many, at what price). They work together during checkout: first create the order
to get its ID, then create each line item referencing that order ID.

Notice CreateOrder returns the new order's ID via res.LastInsertId(). This is
essential because the order items need to know which order they belong to — a
classic one-to-many relationship (one Order → many OrderItems) linked by the
orderId foreign key.
*/

package order

import (
	"database/sql" // stdlib: Go's generic SQL interface

	"github.com/DsThakurRawat/gocom/types"
)

// Store holds the database connection pool — same pattern as user and product stores.
type Store struct {
	db *sql.DB
}

// NewStore — constructor with dependency injection. The DB pool comes from outside.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateOrder inserts an order "header" and returns the new order's auto-generated ID.
// The returned ID is how we link OrderItems to their parent Order.
// db.Exec returns a sql.Result which gives us LastInsertId() — MySQL's way of
// telling us the auto-increment value it assigned.
func (s *Store) CreateOrder(order types.Order) (int, error) {
	res, err := s.db.Exec("INSERT INTO orders (userId, total, status, address) VALUES (?, ?, ?, ?)", order.UserID, order.Total, order.Status, order.Address)
	if err != nil {
		return 0, err
	}

	// LastInsertId returns int64 (because some DBs support huge IDs).
	// We cast to int for convenience since our IDs won't exceed int range.
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

// CreateOrderItem inserts one line item within an order.
// Each item captures a SNAPSHOT of the price at purchase time — so if the product
// price changes later, historical orders still show what the customer actually paid.
// The orderId and productId are foreign keys enforced by the database schema.
func (s *Store) CreateOrderItem(orderItem types.OrderItem) error {
	_, err := s.db.Exec("INSERT INTO order_items (orderId, productId, quantity, price) VALUES (?, ?, ?, ?)", orderItem.OrderID, orderItem.ProductID, orderItem.Quantity, orderItem.Price)
	return err
}

/*
Things to remember from this file
LastInsertId() is MySQL-specific behavior. After an INSERT with an AUTO_INCREMENT
column, MySQL remembers the generated ID and you can retrieve it from the result.
This is how you get the parent ID to link child records (order → order_items).
Not all databases support this the same way — PostgreSQL, for example, prefers
RETURNING clauses.

Price snapshots in order items. The price stored in an order item is the price
AT THE TIME OF PURCHASE, not a reference to the current product price. This is
deliberate: if you change a product's price tomorrow, existing orders should still
reflect what customers actually paid. This is standard e-commerce practice.

Write-only (for now). This store only has INSERT operations. A production version
would add queries like GetOrdersByUserID (for order history), GetOrderByID (for
order details), and UpdateOrderStatus (for fulfillment workflows). These are listed
in IMPROVEMENTS.md as future work.

Error handling shorthand. CreateOrderItem returns the error directly (return err)
instead of the if err != nil { return err } pattern. Both are valid Go — the
shorthand works when you have nothing else to do or return on error.
*/
