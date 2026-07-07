/*



What this file does
This is the payoff file — the HTTP handler layer for users, where every piece you've built so far finally connects. It's the Controller in the classic layered architecture: it receives HTTP requests, orchestrates the work (validate → check DB → hash → create token), and writes back HTTP responses. It doesn't contain business rules or SQL itself — it coordinates the other layers (utils for I/O, auth for tokens/passwords, store for the DB).
Watch how the layers stack up in a single request. A /register call comes in → utils.ParseJSON reads the body → utils.Validate checks the rules → store.GetUserByEmail checks the DB → auth.HashPassword secures the password → store.CreateUser saves it → utils.WriteJSON responds. Each layer does one job. That separation is the whole point of structuring a backend this way.
The most important pattern to notice is the guard-clause / early-return style: every step checks for an error and returns immediately if something's wrong. By the time execution reaches the bottom of a handler, you know everything succeeded. This is idiomatic Go and makes handlers easy to read top-to-bottom.


















*/












package user

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/DsThakurRawat/gocom/configs"
	"github.com/DsThakurRawat/gocom/services/auth"
	"github.com/DsThakurRawat/gocom/types"
	"github.com/DsThakurRawat/gocom/utils"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux" // 3rd-party ROUTER: matches URLs to handlers, extracts path variables
)

// Handler is the controller. It holds the store so its methods can reach the DB.
// Note the type: types.UserStore (the INTERFACE), not *user.Store (the concrete).
// Depending on the interface is what makes this handler testable — you can pass
// in a mock store in tests.
type Handler struct {
	store types.UserStore
}

// Constructor — same dependency-injection pattern as your Store.
func NewHandler(store types.UserStore) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes wires URL paths + HTTP methods to handler functions.
// Keeping routing INSIDE the package (instead of in main.go) means each
// feature owns its own routes — main.go just calls RegisterRoutes.
func (h *Handler) RegisterRoutes(router *mux.Router) {
	// Public routes — anyone can hit these.
	router.HandleFunc("/login", h.handleLogin).Methods("POST")
	router.HandleFunc("/register", h.handleRegister).Methods("POST")

	// PROTECTED route — wrapped in the JWT middleware from your auth package.
	// {userID} is a PATH VARIABLE; mux captures it and you read it via mux.Vars.
	// Only requests with a valid token reach handleGetUser.
	router.HandleFunc("/users/{userID}", auth.WithJWTAuth(h.handleGetUser, h.store)).Methods(http.MethodGet)
}

// ---- LOGIN: verify credentials, hand back a JWT ----
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	// 1. Decode the JSON body into the login payload (email + password).
	var user types.LoginUserPayload
	if err := utils.ParseJSON(r, &user); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err) // 400 = client sent bad data
		return
	}

	// 2. Run the `validate:"..."` rules from the payload struct.
	if err := utils.Validate.Struct(user); err != nil {
		// Type-assert to the validator's error type to format it nicely.
		errors := err.(validator.ValidationErrors)
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid payload: %v", errors))
		return
	}

	// 3. Look up the user by email.
	u, err := h.store.GetUserByEmail(user.Email)
	if err != nil {
		// SECURITY: notice the message says "invalid email OR password" — it does
		// NOT reveal whether the email exists. Telling attackers "this email isn't
		// registered" leaks info (user enumeration). Stay vague on auth failures.
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("not found, invalid email or password"))
		return
	}

	// 4. Compare the submitted plaintext password against the stored HASH.
	//    You never decrypt a hash — you hash the input and compare. bcrypt does
	//    this in a constant-time way to resist timing attacks.
	if !auth.ComparePasswords(u.Password, []byte(user.Password)) {
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid email or password"))
		return
	}

	// 5. Credentials good → mint a JWT and return it. This is the token the
	//    client stores and sends back on future protected requests.
	secret := []byte(configs.Envs.JWTSecret)
	token, err := auth.CreateJWT(secret, u.ID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err) // 500 = OUR fault, not client's
		return
	}

	utils.WriteJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ---- REGISTER: create a brand-new user ----
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	// 1. Decode into the register payload (name, email, password).
	var user types.RegisterUserPayload
	if err := utils.ParseJSON(r, &user); err != nil {
		utils.WriteError(w, http.StatusBadRequest, err)
		return
	}

	// 2. Validate.
	if err := utils.Validate.Struct(user); err != nil {
		errors := err.(validator.ValidationErrors)
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid payload: %v", errors))
		return
	}

	// 3. Ensure the email isn't already taken. Clever inversion here:
	//    if err == nil, that means the lookup SUCCEEDED, i.e. a user DOES exist.
	//    So "no error" is the failure case for registration.
	_, err := h.store.GetUserByEmail(user.Email)
	if err == nil {
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("user with email %s already exists", user.Email))
		return
	}

	// 4. NEVER store a raw password. Hash it (bcrypt) before it touches the DB.
	//    If your database ever leaks, hashed passwords are far harder to crack.
	hashedPassword, err := auth.HashPassword(user.Password)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// 5. Save the new user with the HASH (not the plaintext) as the password.
	err = h.store.CreateUser(types.User{
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Email:     user.Email,
		Password:  hashedPassword,
	})
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	// 6. 201 Created — the correct status for "a new resource was made".
	//    Body is nil; nothing to return.
	utils.WriteJSON(w, http.StatusCreated, nil)
}

// ---- GET USER: protected read (only reachable through the JWT middleware) ----
func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	// mux.Vars pulls path variables captured by the {userID} pattern.
	vars := mux.Vars(r)
	str, ok := vars["userID"]
	if !ok {
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("missing user ID"))
		return
	}

	// URL segments are strings; convert to int.
	userID, err := strconv.Atoi(str)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, fmt.Errorf("invalid user ID"))
		return
	}

	// Fetch and return the user. Recall Password has `json:"-"`, so it's
	// automatically stripped from this JSON response — no accidental leak.
	user, err := h.store.GetUserByID(userID)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	utils.WriteJSON(w, http.StatusOK, user)
}
/*


Things to remember from this file
The layered request pipeline. Handler → validate → store → auth → respond. Each layer has one responsibility, and the handler is just the conductor. This is the mental model for almost every endpoint you'll ever write.
Guard clauses everywhere. Check error, return early. It keeps the happy path flat and readable, and guarantees that reaching the end means success.
HTTP status codes carry meaning. 400 = client's fault (bad/invalid input), 401/403 = auth problems, 404 = not found, 201 = created, 500 = your server's fault. Using the right code is part of a well-designed API, not decoration — clients branch on these.
Password security done right. Two habits here are gold-standard: hash before storing (HashPassword), and compare hash-to-input at login (ComparePasswords) rather than ever storing or comparing plaintext.
Don't leak information in auth errors. The vague "invalid email or password" is deliberate. Precise errors ("no such email") let attackers enumerate which accounts exist. Small wording, real security impact.
Routes live with their feature. Each handler package exposes RegisterRoutes, so main.go stays tiny. This scales — imagine adding product, order, cart packages, each self-registering.
A couple of things to flag as you sharpen up:
The "already exists" check has a race condition. Between step 3 (check email) and step 5 (insert), two simultaneous requests could both pass the check and both insert. The real safety net is a UNIQUE constraint on the email column in the database — then even if the check races, the DB rejects the duplicate. Treat the app-level check as a nicety, the DB constraint as the guarantee.
Type assertions can panic. err.(validator.ValidationErrors) assumes the error is always that type. If Validate.Struct ever returned a different error kind, this would crash. In hardened code you'd use the comma-ok form. Minor here, but a good instinct to build.
You've now walked the full vertical slice: types (models) → store (DB) → auth (security) → handler (HTTP). That's a complete, well-structured Go backend feature. The natural next files are the same four-layer pattern for products or orders/cart, or the main.go that boots the server, opens the DB connection, and calls all these RegisterRoutes. Which do you want to tackle next?















*/