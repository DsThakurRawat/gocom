package cart

import (
	"fmt"
	"net/http"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/DsThakurRawat/gocom/services/auth"
	"github.com/DsThakurRawat/gocom/types"
	"github.com/DsThakurRawat/gocom/utils"
)

// Handler holds THREE stores — checkout is a cross-cutting workflow:
//   store       → look up product details & prices
//   orderStore  → create the order + order items
//   userStore   → needed by the JWT middleware to verify the user
// Depending on the INTERFACES (not concrete types) keeps this testable.
type Handler struct {
	store      types.ProductStore
	orderStore types.OrderStore
	userStore  types.UserStore
}

// Constructor takes all three dependencies and injects them in.
func NewHandler(
	store types.ProductStore,
	orderStore types.OrderStore,
	userStore types.UserStore,
) *Handler {
	return &Handler{
		store:      store,
		orderStore: orderStore,
		userStore:  userStore,
	}
}

// One protected route: you must be logged in to check out.
// Wrapped in WithJWTAuth so a valid token is required, and the middleware
// stashes the user's ID into the request context for us to read.
func (h *Handler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/cart/checkout", auth.WithJWTAuth(h.handleCheckout, h.userStore)).Methods(http.MethodPost)
}

func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	// 1. WHO is checking out? Pull the user ID the auth middleware placed in
	//    the context. This is the payoff of that context work in the auth file —
	//    the handler knows the user WITHOUT trusting anything the client sent.
	userID := auth.GetUserIDFromContext(r.Context())

	// 2. Decode the cart body: a list of {productID, quantity} items.
	var cart types.CartCheckoutPayload
	if err := utils.ParseJSON(r, &cart); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err)
		return
	}

	// 3. Validate — the `validate:"required"` on Items ensures the cart isn't empty.
	if err := utils.Validate.Struct(cart); err != nil {
		errors := err.(validator.ValidationErrors)
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid payload: %v", errors))
		return
	}

	// 4. Extract just the product IDs from the cart items. This helper
	//    (in the sibling cart.go file) also guards against quantity <= 0.
	productIds, err := getCartItemsIDs(cart.Items)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err)
		return
	}

	// 5. Fetch all those products in ONE query (GetProductsByID — the N+1
	//    avoidance from your product store). Now we have real prices & stock.
	products, err := h.store.GetProductsByID(productIds)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// 6. The core business logic (in cart.go): check stock, compute the total
	//    from SERVER-SIDE prices, create the order + order items, decrement
	//    inventory. Returns the new order's ID and the calculated total.
	//    KEY SECURITY POINT: the price comes from OUR products, never from the
	//    client's payload — a client can only choose product+quantity, not price.
	orderID, totalPrice, err := h.createOrder(products, cart.Items, userID)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, err)
		return
	}

	// 7. Success — return the order id and total.
	//    map[string]interface{} lets us mix a float (total) and int (id) in
	//    one JSON object without defining a dedicated response struct.
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"total_price": totalPrice,
		"order_id":    orderID,
	})
}