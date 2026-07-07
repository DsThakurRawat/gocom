/*
What this file does
This is the product HTTP handler — the Controller layer for product-related
endpoints. It follows the exact same pattern as the user handler: a Handler
struct holds the stores it needs, a constructor injects them, RegisterRoutes
wires paths to methods, and each handler method does the Parse → Validate →
Store → Respond pipeline.

Three routes live here:
  GET  /products            → list all products (public)
  GET  /products/{productID} → get one product by ID (public)
  POST /products            → create a new product (JWT-protected / admin)

The GET routes are public — anyone can browse products. The POST route is wrapped
in auth.WithJWTAuth, meaning only authenticated users can create products. In a
real app you'd also check for an "admin" role, but this project keeps it simple
with just token-based access control.

Notice this handler holds TWO stores: ProductStore (for the actual CRUD) and
UserStore (needed by the JWT middleware to verify the token's user exists in the DB).
*/

package product

import (
	"fmt"
	"net/http"
	"strconv" // string → int conversion for URL path variables

	"github.com/go-playground/validator/v10" // validates the `validate:"..."` struct tags
	"github.com/gorilla/mux"                 // router with path variable extraction
	"github.com/DsThakurRawat/gocom/services/auth"
	"github.com/DsThakurRawat/gocom/types"
	"github.com/DsThakurRawat/gocom/utils"
)

// Handler holds both stores — ProductStore for CRUD, UserStore for auth middleware.
type Handler struct {
	store     types.ProductStore
	userStore types.UserStore
}

// Constructor — injects both dependencies.
func NewHandler(store types.ProductStore, userStore types.UserStore) *Handler {
	return &Handler{store: store, userStore: userStore}
}

// RegisterRoutes wires URL patterns to handler methods.
// GET routes are public; POST is protected by JWT middleware.
func (h *Handler) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/products", h.handleGetProducts).Methods(http.MethodGet)
	router.HandleFunc("/products/{productID}", h.handleGetProduct).Methods(http.MethodGet)

	// admin routes — only authenticated users can create products.
	// In production you'd also check for an "admin" role.
	router.HandleFunc("/products", auth.WithJWTAuth(h.handleCreateProduct, h.userStore)).Methods(http.MethodPost)
}

// handleGetProducts returns the entire product catalog.
// No auth required — this is a public storefront endpoint.
func (h *Handler) handleGetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.store.GetProducts()
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	utils.WriteJSON(w, http.StatusOK, products)
}

// handleGetProduct fetches a single product by its ID from the URL path.
// {productID} is a PATH VARIABLE captured by gorilla/mux.
func (h *Handler) handleGetProduct(w http.ResponseWriter, r *http.Request) {
	// mux.Vars(r) returns a map of all path variables.
	vars := mux.Vars(r)
	str, ok := vars["productID"]
	if !ok {
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("missing product ID"))
		return
	}

	// URL path segments are always strings — convert to int.
	productID, err := strconv.Atoi(str)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid product ID"))
		return
	}

	product, err := h.store.GetProductByID(productID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	utils.WriteJSON(w, http.StatusOK, product)
}

// handleCreateProduct adds a new product to the catalog.
// Protected by JWT — only authenticated users reach this handler.
// Uses the CreateProductPayload (not the full Product struct) so the client
// can only set allowed fields (no ID, no CreatedAt).
func (h *Handler) handleCreateProduct(w http.ResponseWriter, r *http.Request) {
	var product types.CreateProductPayload
	if err := utils.ParseJSON(r, &product); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err)
		return
	}

	// Validate using the `validate:"required"` tags on the payload struct.
	if err := utils.Validate.Struct(product); err != nil {
		errors := err.(validator.ValidationErrors)
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid payload: %v", errors))
		return
	}

	err := h.store.CreateProduct(product)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// 201 Created — the correct status for "a new resource was made."
	utils.WriteJSON(w, http.StatusCreated, product)
}

/*
Things to remember from this file
The same handler pattern, again. Parse → Validate → Store → Respond. Once you
internalize this pipeline, you can write any CRUD endpoint in your sleep. The
product handler is structurally identical to the user handler — different data,
same flow.

Public vs protected routes. GET endpoints are open (anyone browses the catalog);
POST is behind JWT auth (only logged-in users create products). The auth decision
happens at the ROUTE REGISTRATION level — clean and declarative.

Path variables with gorilla/mux. The {productID} in the route pattern is extracted
via mux.Vars(r). Always convert it from string to the expected type (strconv.Atoi
for ints) and handle the conversion error — a malicious client could send
"/products/abc" instead of "/products/42".

Payload vs Model types. handleCreateProduct uses CreateProductPayload (limited
client-settable fields) not Product (which includes ID, CreatedAt). This is the
DTO pattern — controlling exactly what the client can set. The DB generates ID
and CreatedAt automatically.
*/
