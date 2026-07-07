package utils

import (
	"encoding/json" // stdlib: converts between Go values and JSON
	"fmt"
	"net/http" // stdlib: the core HTTP server types (Request, ResponseWriter)

	"github.com/go-playground/validator/v10" // 3rd-party: enforces the `validate:"..."` struct tags
)

// Validate is a single, shared validator instance created ONCE at startup.
// The library caches reflection info internally, so you reuse this one object
// everywhere rather than calling validator.New() on every request (which would
// be wasteful). Handlers call utils.Validate.Struct(payload) to check rules.
var Validate = validator.New()

// WriteJSON is the standard way to send ANY successful response.
// w = the connection back to the client. status = HTTP code (200, 201...). v = any Go value.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	// ORDER MATTERS in HTTP responses. You must set headers first...
	w.Header().Add("Content-Type", "application/json") // tells the client "this is JSON"
	// ...then the status code...
	w.WriteHeader(status) // once this is called, headers are locked/sent
	// ...then the body. Encoder streams the JSON straight to the connection.
	return json.NewEncoder(w).Encode(v)
	// `any` (alias for interface{}) = "accepts any type" — that's why one
	// function can serialize a User, a Product, an error map, anything.
}

// WriteError is a thin wrapper over WriteJSON for the failure case.
// It guarantees every error your API returns has the SAME shape:
//   { "error": "some message" }
// Consistent error formats make life easy for whoever consumes your API.
func WriteError(w http.ResponseWriter, status int, err error) {
	WriteJSON(w, status, map[string]string{"error": err.Error()})
	// err.Error() extracts the human-readable string from the error value.
}

// ParseJSON reads an INCOMING request body and decodes it into `v`.
// You pass a POINTER (e.g. &payload) so Decode can fill your struct in place.
func ParseJSON(r *http.Request, v any) error {
	if r.Body == nil {
		// Defensive check: a GET request, or a malformed one, may have no body.
		// fmt.Errorf creates a new error value with your custom message.
		return fmt.Errorf("missing request body")
	}
	// Decoder reads the request stream and maps JSON keys → struct fields
	// (using the `json:"..."` tags). This is the reverse of WriteJSON.
	return json.NewDecoder(r.Body).Decode(v)
}

// GetTokenFromRequest extracts a JWT auth token from an incoming request.
// It supports TWO common ways clients send tokens, checked in priority order:
func GetTokenFromRequest(r *http.Request) string {
	// 1. The standard, preferred way: the "Authorization" HTTP header.
	tokenAuth := r.Header.Get("Authorization")
	// 2. A fallback: a "?token=..." query parameter in the URL.
	//    (Handy for things like links, but less secure — query strings can
	//    end up in server logs and browser history, so headers are preferred.)
	tokenQuery := r.URL.Query().Get("token")

	if tokenAuth != "" {
		return tokenAuth
	}
	if tokenQuery != "" {
		return tokenQuery
	}
	return "" // no token found — caller treats this as "unauthenticated"
}

/*

What this file does
This is your utilities / helpers file — a collection of small, reusable functions that your HTTP handlers call over and over. Instead of writing "encode this to JSON and set headers" in every single handler, you write it once here and reuse it everywhere. This keeps handlers short and consistent (the DRY principle — Don't Repeat Yourself).
There are really four helpers doing three jobs: writing JSON responses out (WriteJSON, WriteError), reading JSON requests in (ParseJSON), and pulling the auth token out of an incoming request (GetTokenFromRequest). Plus one shared Validate object used to enforce those validate:"required" tags you saw in your types package.
Here's the key backend concept running through it all: an HTTP request/response is fundamentally just text over a wire. Your Go code works with structs; the client works with JSON. These functions are the translation layer between the two — structs → JSON on the way out, JSON → structs on the way in.
*/

/*
The concepts worth remembering as a backend dev
The big three ideas here are the input → process → output cycle of every HTTP endpoint. ParseJSON is input, your handler is process, WriteJSON/WriteError is output.
A few things specifically worth internalizing:
Pointers for decoding. ParseJSON(r, &payload) passes the address of your struct so the function can write into it. If you passed the struct by value, the decoded data would vanish when the function returns. This trips up almost everyone at first.
any / interface{}. This is what lets these helpers be generic — one WriteJSON handles every response type in your whole app. It's the Go equivalent of "accepts anything."
HTTP response ordering. Headers → status code → body, always in that sequence. Calling w.WriteHeader() after writing the body is a classic bug that produces a confusing runtime warning.
Consistent error shape. By funneling all errors through WriteError, every failure looks identical to the client. Real APIs live or die by this consistency.
One small production note: GetTokenFromRequest returns the raw Authorization header, which usually looks like "Bearer eyJhbGc...". Most real code strips the "Bearer " prefix before verifying — so don't be surprised when your JWT-validation step later needs to do strings.TrimPrefix(token, "Bearer ").

*/
