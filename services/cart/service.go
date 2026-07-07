/*
What this file does
This is the cart business logic layer — the core "engine" behind checkout. While
routes.go handles HTTP concerns (parse request, validate, respond), THIS file
answers the actual domain questions: "Is everything in stock?" → "What's the
total?" → "Create the order + items." → "Decrement inventory."

The key design here is that the Handler methods call these pure-logic helpers,
keeping the HTTP layer thin and the business rules testable in isolation. If you
wanted to unit-test checkout logic without spinning up a server, you'd test
these functions directly.

The createOrder method is the orchestrator: it builds a product map for O(1)
lookups, checks stock, calculates totals from SERVER-SIDE prices (never trusting
the client), updates inventory, and creates the order + line items in the DB.
Every price comes from your database, not from what the client sent — that's a
critical e-commerce security principle.
*/

package cart

import (
	"fmt"

	"github.com/DsThakurRawat/gocom/types"
)

// getCartItemsIDs extracts product IDs from the checkout items.
// Also serves as the first validation gate: quantity must be positive.
// If someone sends quantity=0 or negative, we reject early — no point
// looking up products for an invalid cart.
func getCartItemsIDs(items []types.CartCheckoutItem) ([]int, error) {
	productIds := make([]int, len(items))
	for i, item := range items {
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("invalid quantity for product %d", item.ProductID)
		}

		productIds[i] = item.ProductID
	}

	return productIds, nil
}

// checkIfCartIsInStock verifies every cart item exists and has enough stock.
// The products map gives O(1) lookups — much better than scanning a slice
// for each cart item (which would be O(n*m)).
func checkIfCartIsInStock(cartItems []types.CartCheckoutItem, products map[int]types.Product) error {
	if len(cartItems) == 0 {
		return fmt.Errorf("cart is empty")
	}

	for _, item := range cartItems {
		product, ok := products[item.ProductID]
		if !ok {
			return fmt.Errorf("product %d is not available in the store, please refresh your cart", item.ProductID)
		}

		if product.Quantity < item.Quantity {
			return fmt.Errorf("product %s is not available in the quantity requested", product.Name)
		}
	}

	return nil
}

// calculateTotalPrice uses SERVER-SIDE prices from the database.
// SECURITY: the client only sends productID + quantity. We look up
// the real price ourselves. If you trusted client-sent prices, anyone
// could buy a $1000 item for $1 by editing the request.
func calculateTotalPrice(cartItems []types.CartCheckoutItem, products map[int]types.Product) float64 {
	var total float64

	for _, item := range cartItems {
		product := products[item.ProductID]
		total += product.Price * float64(item.Quantity)
	}

	return total
}

// createOrder is the orchestrator — the "conductor" of the checkout workflow.
// It chains together: build map → check stock → calc total → update inventory →
// create order → create line items. Each step depends on the previous succeeding.
func (h *Handler) createOrder(products []types.Product, cartItems []types.CartCheckoutItem, userID int) (int, float64, error) {
	// create a map of products for easier access
	productsMap := make(map[int]types.Product)
	for _, product := range products {
		productsMap[product.ID] = product
	}

	// check if all products are available
	if err := checkIfCartIsInStock(cartItems, productsMap); err != nil {
		return 0, 0, err
	}

	// calculate total price
	totalPrice := calculateTotalPrice(cartItems, productsMap)

	// reduce the quantity of products in the store
	// NOTE: This is NOT atomic — two simultaneous checkouts could oversell.
	// A production fix would use DB transactions + row locks (SELECT ... FOR UPDATE).
	for _, item := range cartItems {
		product := productsMap[item.ProductID]
		product.Quantity -= item.Quantity
		h.store.UpdateProduct(product)
	}

	// create order record
	orderID, err := h.orderStore.CreateOrder(types.Order{
		UserID:  userID,
		Total:   totalPrice,
		Status:  "pending",
		Address: "some address", // could fetch address from a user addresses table
	})
	if err != nil {
		return 0, 0, err
	}

	// create order the items records
	// Each line item snapshots the price AT TIME OF PURCHASE so later
	// price changes don't rewrite order history.
	for _, item := range cartItems {
		h.orderStore.CreateOrderItem(types.OrderItem{
			OrderID:   orderID,
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     productsMap[item.ProductID].Price,
		})
	}

	return orderID, totalPrice, nil
}

/*
Things to remember from this file
The separation of concerns pattern. routes.go handles HTTP; this file handles
business logic. Neither knows about the other's internals. This makes the
business rules testable without HTTP, and the HTTP layer swappable without
touching the rules.

Server-side price calculation. The client sends {productID, quantity}. The server
looks up the real price. This is a MUST for any e-commerce system — trusting
client prices is the #1 checkout vulnerability.

The non-atomic inventory update. Each product.Quantity -= item.Quantity happens
outside a transaction. Two simultaneous checkouts could both read stock=1 and
both succeed (classic race condition / overselling). The fix: wrap everything
in a DB transaction with row-level locks. This is noted in IMPROVEMENTS.md.

Map for O(1) lookups. Converting the product slice to a map[int]types.Product
turns repeated lookups from O(n) to O(1). A small optimization that matters
when carts grow or when you're handling many concurrent checkouts.
*/