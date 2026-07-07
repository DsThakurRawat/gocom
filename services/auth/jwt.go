/*


What this file does
This is your authentication layer — specifically JWT (JSON Web Token) auth. It answers two questions every protected API needs: "how do I give a logged-in user proof of who they are?" (CreateJWT), and "how do I check that proof on every future request?" (WithJWTAuth).
Here's the whole story. When a user logs in successfully, you call CreateJWT to generate a token — a signed string that secretly encodes their user ID. You hand that token to the client. From then on, the client sends it back with every request. Your middleware (WithJWTAuth) intercepts protected requests, verifies the token is real and untampered, looks up the actual user, and only then lets the real handler run. If anything is off, it stops with "permission denied."
The magic that makes JWTs trustworthy is the cryptographic signature. The token is signed with your secret JWTSecret. If anyone changes even one character of the token (say, to impersonate another user), the signature no longer matches and verification fails. The server doesn't need to store tokens in a database — it just re-checks the signature. That's the whole appeal of JWTs: stateless authentication.
Two other backend concepts appear here. Middleware is a function that wraps another handler to run logic before (or after) it — perfect for auth checks you want on many routes without copy-pasting. And context is Go's standard way to carry request-scoped values (like "who is the logged-in user") from the middleware down into the handler.





*/
package auth

import (
	"context" // stdlib: carries request-scoped values (here, the logged-in user's ID)
	"fmt"
	"log"
	"net/http"
	"strconv" // string <-> int conversion (JWT claims are stored as strings here)
	"time"

	"github.com/golang-jwt/jwt/v5" // 3rd-party: the JWT create/parse library
	"github.com/DsThakurRawat/gocom/configs"
	"github.com/DsThakurRawat/gocom/types"
	"github.com/DsThakurRawat/gocom/utils"
)

// contextKey is a custom string type used as a KEY for context values.
// Why a custom type instead of a plain string? To avoid collisions: if two
// packages both used the raw string "userID" as a context key, they'd clash.
// A private named type guarantees YOUR key is unique. This is the idiomatic
// Go pattern for context keys.
type contextKey string

// UserKey is the specific key under which we store the user's ID in the context.
const UserKey contextKey = "userID"

// WithJWTAuth is MIDDLEWARE. It takes the real handler you want to protect,
// wraps it in auth checks, and returns a NEW handler. The gatekeeper pattern:
// only if every check passes does the wrapped handlerFunc actually run.
// It also takes a UserStore so it can verify the user still exists in the DB.
func WithJWTAuth(handlerFunc http.HandlerFunc, store types.UserStore) http.HandlerFunc {
	// Returning a closure — a function that "remembers" handlerFunc and store.
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Pull the raw token string out of the request (header or query).
		tokenString := utils.GetTokenFromRequest(r)

		// 2. Verify the signature & structure of the token.
		token, err := validateJWT(tokenString)
		if err != nil {
			log.Printf("failed to validate token: %v", err)
			permissionDenied(w) // stop here — never call the real handler
			return
		}
		// 3. Even a parseable token can be invalid (e.g. expired/tampered).
		if !token.Valid {
			log.Println("invalid token")
			permissionDenied(w)
			return
		}

		// 4. Extract the "claims" — the data payload we baked into the token.
		//    `.(jwt.MapClaims)` is a TYPE ASSERTION: claims is a generic
		//    interface, and we assert it's actually a MapClaims (a map).
		claims := token.Claims.(jwt.MapClaims)
		str := claims["userID"].(string) // we stored userID as a string, so assert string

		// 5. Convert that string back into an int (Atoi = "ASCII to int").
		userID, err := strconv.Atoi(str)
		if err != nil {
			log.Printf("failed to convert userID to int: %v", err)
			permissionDenied(w)
			return
		}

		// 6. Confirm the user actually exists in the database. A valid token
		//    for a deleted user should still be rejected.
		u, err := store.GetUserByID(userID)
		if err != nil {
			log.Printf("failed to get user by id: %v", err)
			permissionDenied(w)
			return
		}

		// 7. Stash the user's ID into the request's context so the real
		//    handler downstream can retrieve it (via GetUserIDFromContext).
		//    Contexts are immutable — WithValue returns a NEW context, and
		//    r.WithContext returns a NEW request. You must reassign both.
		ctx := r.Context()
		ctx = context.WithValue(ctx, UserKey, u.ID)
		r = r.WithContext(ctx)

		// 8. All checks passed — finally run the protected handler.
		handlerFunc(w, r)
	}
}

// CreateJWT generates a fresh signed token for a given user, called at login.
func CreateJWT(secret []byte, userID int) (string, error) {
	// How long the token stays valid, pulled from your config.
	expiration := time.Second * time.Duration(configs.Envs.JWTExpirationInSeconds)

	// Build the token: choose a signing ALGORITHM (HS256 = HMAC + SHA-256,
	// a symmetric scheme using one shared secret) and attach CLAIMS (payload).
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userID":    strconv.Itoa(int(userID)),        // who the token belongs to
		"expiresAt": time.Now().Add(expiration).Unix(), // when it stops being valid (unix timestamp)
	})

	// SignedString applies the secret to produce the final token string.
	// This signing step is what makes the token tamper-proof.
	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", err
	}
	return tokenString, err
}

// validateJWT parses AND verifies a token string.
func validateJWT(tokenString string) (*jwt.Token, error) {
	// jwt.Parse needs a callback that returns the KEY to verify the signature.
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// SECURITY CHECK: confirm the token was signed with the algorithm we
		// expect (HMAC). Without this, an attacker could swap the algorithm
		// (the classic "alg=none" / algorithm-confusion attack) to forge tokens.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Return our secret so the library can recompute & compare the signature.
		return []byte(configs.Envs.JWTSecret), nil
	})
}

// permissionDenied is a small helper for the standard 403 rejection response.
// 403 Forbidden = "I know who you are (or don't), but you can't do this."
func permissionDenied(w http.ResponseWriter) {
	utils.WriteError(w, http.StatusForbidden, fmt.Errorf("permission denied"))
}

// GetUserIDFromContext is how a protected handler ASKS "who is logged in?"
// It reads the ID the middleware stashed in step 7 above.
func GetUserIDFromContext(ctx context.Context) int {
	// Type-assert with the comma-ok form so a missing/wrong value doesn't panic.
	userID, ok := ctx.Value(UserKey).(int)
	if !ok {
		return -1 // sentinel value meaning "no authenticated user found"
	}
	return userID
}

/*

The concepts worth remembering
Middleware is the star here. The shape func(handler) handler — take a handler, return a wrapped handler — is the pattern for cross-cutting concerns: auth, logging, rate-limiting, CORS. You'll write dozens of these. In your router you'd use it like router.HandleFunc("/cart", auth.WithJWTAuth(handleCart, userStore)).
Stateless auth via signatures. The server stores nothing per-token. Trust comes purely from the secret-based signature. Flip side: because there's no server-side record, you can't easily "revoke" a JWT before it expires — a known trade-off you'll deal with later (short expiry times, token blacklists, etc.).
Type assertions (x.(SomeType)) show up a lot with JWTs because claims come back as a generic map. Always prefer the comma-ok form (v, ok := x.(T)) in production so a bad value returns an error instead of crashing your server with a panic — notice GetUserIDFromContext does this safely, while the middleware uses the risky non-ok form on claims["userID"].(string).
Context for request-scoped data. The middleware injects the user ID; the handler reads it back out. This is how Go passes "who's making this request" down the call chain without adding it to every function signature.
Two things to flag for later, since you're learning the real craft:
The "expiresAt" claim here is set but not actually enforced — the standard registered claim for expiry is "exp", and the jwt library auto-checks that one. As written, using the custom key "expiresAt" means expired tokens might still pass token.Valid. Worth revisiting when you harden this.
And GetTokenFromRequest returns the header raw, so if a client sends Authorization: Bearer <token>, the "Bearer " prefix is still attached and parsing may fail — you'd typically strip it before validateJWT.
Want to move on to the login/register handler next? That's where CreateJWT and your password hashing actually get called — it ties this whole auth story together.











*/