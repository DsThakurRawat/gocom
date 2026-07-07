/*
What this file does
This is the API server — the "glue" that wires every feature package (user,
product, cart, order) into a single running HTTP server. It's the central
nervous system of your app's routing.

The pattern here is called "service locator" or more precisely "composition":
the Run() method creates all the stores (data access), creates all the handlers
(HTTP controllers), and tells each handler to register its own routes on a
shared router. Each feature package is self-contained — it owns its own routes,
handlers, and store — and this file just orchestrates the assembly.

The subrouter with PathPrefix("/api/v1") means all your API routes live under
/api/v1/... (e.g. /api/v1/login, /api/v1/products). This is API VERSIONING —
if you ever need breaking changes, you create /api/v2 without disrupting v1
clients. It's a best practice for any public-facing API.

Notice the dependency injection chain: each handler receives the stores it needs
via its constructor (NewHandler). The cart handler, for example, needs THREE
stores (product, order, user) because checkout touches all three domains.
*/

package api

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/gorilla/mux"                        // 3rd-party: powerful HTTP router with path variables & subrouters
	"github.com/DsThakurRawat/gocom/services/cart"    // cart checkout handler
	"github.com/DsThakurRawat/gocom/services/order"   // order data access (store only, no HTTP handler)
	"github.com/DsThakurRawat/gocom/services/product" // product CRUD handler
	"github.com/DsThakurRawat/gocom/services/user"    // user auth handler (login/register)
)

// APIServer holds the listen address and the database connection pool.
// These are the two things every handler eventually needs access to.
type APIServer struct {
	addr string
	db   *sql.DB
}

// NewAPIServer is the constructor — same DI pattern you've seen everywhere.
func NewAPIServer(addr string, db *sql.DB) *APIServer {
	return &APIServer{
		addr: addr,
		db:   db,
	}
}

// Run is where the magic happens. It:
//   1. Creates a gorilla/mux router
//   2. Sets up an /api/v1 subrouter (API versioning)
//   3. Creates stores + handlers for each feature
//   4. Lets each handler register its own routes
//   5. Starts the HTTP server (blocks forever)
func (s *APIServer) Run() error {
	router := mux.NewRouter()
	// PathPrefix creates a subrouter — all routes registered on it
	// automatically get "/api/v1" prepended. Clean URL namespacing.
	subrouter := router.PathPrefix("/api/v1").Subrouter()

	// --- USER feature ---
	// Store handles DB operations; Handler handles HTTP; RegisterRoutes wires paths.
	userStore := user.NewStore(s.db)
	userHandler := user.NewHandler(userStore)
	userHandler.RegisterRoutes(subrouter)

	// --- PRODUCT feature ---
	// Product handler also needs userStore for JWT-protected admin routes
	// (the auth middleware looks up the user to verify they exist).
	productStore := product.NewStore(s.db)
	productHandler := product.NewHandler(productStore, userStore)
	productHandler.RegisterRoutes(subrouter)

	// --- ORDER + CART feature ---
	// Order store has no HTTP handler of its own — it's consumed by the cart.
	// Cart handler needs all three stores: products (to look up prices/stock),
	// orders (to create the order), and users (for JWT auth middleware).
	orderStore := order.NewStore(s.db)

	cartHandler := cart.NewHandler(productStore, orderStore, userStore)
	cartHandler.RegisterRoutes(subrouter)

	// Serve static files (HTML, CSS, JS) from a "static" directory.
	// The "/" catch-all MUST come after API routes, or it would swallow them.
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("static")))

	log.Println("Listening on", s.addr)

	// ListenAndServe blocks forever, handling requests concurrently via goroutines.
	// It only returns on fatal errors (e.g. port already in use).
	return http.ListenAndServe(s.addr, router)
}

/*
Things to remember from this file
API versioning with subrouters. PathPrefix("/api/v1").Subrouter() namespaces all
routes under /api/v1/. When you need breaking changes, add a /api/v2 subrouter
alongside v1 — existing clients keep working. This is standard practice for
production APIs.

Dependency injection in action. Each handler gets exactly the stores it needs,
passed through its constructor. Cart needs 3 stores; user needs 1. No handler
creates its own dependencies — they're all assembled here, making them testable
with mock stores.

The order package has no handler. Not every domain concept needs HTTP routes.
The order store is "internal" — only consumed by the cart handler during checkout.
This is a judgment call: you might add order routes later (GET /orders for order
history), but YAGNI ("You Ain't Gonna Need It") says wait until you do.

Route ordering matters. The static file server with PathPrefix("/") is a catch-all.
If registered before the API routes, it would match everything. Always register
specific routes before catch-alls.
*/
