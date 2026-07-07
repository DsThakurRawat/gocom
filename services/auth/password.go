/*
What this file does
This is the password security layer — two small but CRITICAL functions that handle
password hashing and comparison using bcrypt. These are the building blocks of
secure authentication.

The key concept: you NEVER store a raw password in the database. Instead, you run
it through a one-way hash function (bcrypt) that produces a scrambled, irreversible
string. When a user logs in, you hash their input and compare it to the stored hash.
If your database ever leaks, attackers get hashes — not passwords.

bcrypt is the go-to choice for password hashing because it's deliberately SLOW.
Regular hash functions (SHA-256, MD5) are designed to be fast — great for checksums,
terrible for passwords because attackers can brute-force billions of guesses per
second. bcrypt uses a configurable "cost factor" (bcrypt.DefaultCost = 10, meaning
2^10 = 1024 internal iterations) that makes each hash take ~100ms. That's invisible
to a user logging in, but crippling to an attacker trying millions of guesses.

CompareHashAndPassword is also designed to be constant-time — it takes the same
amount of time whether the password is right or wrong. This prevents "timing attacks"
where an attacker measures response times to guess passwords character by character.
*/

package auth

import (
	"golang.org/x/crypto/bcrypt" // 3rd-party (golang.org/x/): the standard bcrypt implementation for Go
)

// HashPassword takes a plaintext password and returns its bcrypt hash.
// Called during registration — the hash is what gets stored in the database.
// bcrypt.GenerateFromPassword handles the salt internally — each hash includes
// a random salt, so even identical passwords produce different hashes.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePasswords checks if a plaintext password matches a stored bcrypt hash.
// Called during login — you never "decrypt" a hash (that's impossible by design).
// Instead, bcrypt re-hashes the input with the same salt (embedded in the hash
// string) and compares the results.
// Returns true if they match, false otherwise.
func ComparePasswords(hashed string, plain []byte) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), plain)
	return err == nil
}

/*
Things to remember from this file
Never store plaintext passwords. This is the #1 rule of backend security.
HashPassword on registration, ComparePasswords on login. The database only
ever sees the bcrypt hash.

bcrypt is intentionally slow. The DefaultCost of 10 means each hash takes
~100ms. That's fine for one user logging in, but makes brute-force attacks
computationally infeasible (100ms × 10 billion guesses = 31 years).

Salt is automatic. bcrypt generates a random salt for each hash and embeds
it in the output string. Two users with the same password get different hashes.
This defeats precomputed "rainbow table" attacks.

The function signature uses []byte for the plaintext. bcrypt works with byte
slices internally, so the caller converts the password string to []byte before
passing it in. This is just Go's type system at work — strings and []byte are
freely convertible but distinct types.
*/