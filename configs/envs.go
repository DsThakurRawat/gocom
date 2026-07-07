package configs

import (
	"fmt"
	"os"      // stdlib: gives access to os.LookupEnv — reads environment variables
	"strconv" // stdlib: string→number conversion (env vars are ALWAYS strings)

	"github.com/joho/godotenv" // 3rd-party: loads a .env file into the environment
)

// Config is a single typed struct holding every setting the app needs.
// Grouping config into one struct (instead of scattered global strings) means
// the rest of your code gets autocomplete + compile-time safety: configs.Envs.Port
type Config struct {
	PublicHost             string
	Port                   string
	DBUser                 string
	DBPassword             string
	DBAddress              string
	DBName                 string
	JWTSecret              string // secret key used to sign/verify JWT auth tokens — keep private!
	JWTExpirationInSeconds int64  // how long a login token stays valid
}

// Envs is a PACKAGE-LEVEL variable initialized once, at program startup,
// the first time this package is imported. After that, `configs.Envs` is a
// ready-to-use, read-only-by-convention global config accessible app-wide.
// This is a common Go pattern for app configuration.
var Envs = initConfig()

// initConfig builds the Config by reading each value from the environment.
func initConfig() Config {
	// Loads variables from a .env file into the OS environment (for local dev).
	// If no .env exists (e.g. in production), it silently does nothing —
	// which is fine, because real servers inject env vars directly.
	godotenv.Load()

	return Config{
		PublicHost:  getEnv("PUBLIC_HOST", "http://localhost"),
		Port:        getEnv("PORT", "8080"),
		DBUser:      getEnv("DB_USER", "root"),
		DBPassword:  getEnv("DB_PASSWORD", "mypassword"), // fallback ONLY for local dev
		// Builds "host:port" (e.g. "127.0.0.1:3306") by combining two env vars.
		// fmt.Sprintf returns a formatted string instead of printing it.
		DBAddress: fmt.Sprintf("%s:%s", getEnv("DB_HOST", "127.0.0.1"), getEnv("DB_PORT", "3306")),
		DBName:    getEnv("DB_NAME", "ecom"),
		JWTSecret: getEnv("JWT_SECRET", "not-so-secret-now-is-it?"), // MUST be overridden in prod
		// 3600*24*7 = seconds in 7 days. Computed at compile time for readability.
		JWTExpirationInSeconds: getEnvAsInt("JWT_EXPIRATION_IN_SECONDS", 3600*24*7),
	}
}

// getEnv returns the env var for `key`, or `fallback` if it isn't set.
// The comma-ok idiom `value, ok := ...` is core Go: `ok` is true only when
// the key actually exists (distinguishing "set but empty" from "not set").
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// getEnvAsInt does the same but converts the string to an int64.
// Needed because ALL environment variables come in as strings.
func getEnvAsInt(key string, fallback int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		// ParseInt(str, base=10, bitSize=64). Returns the number AND an error.
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			// Value existed but wasn't a valid number → fall back safely
			// instead of crashing the app.
			return fallback
		}
		return i
	}
	return fallback
}

/*
What this file does
This is your configuration loader. Its whole job is to read settings from environment variables and make them available to the rest of your app in one clean, typed place (configs.Envs).
The core idea — called the 12-Factor App principle of "config in the environment" — is that things like database passwords, ports, and secret keys should never be hardcoded in your source. Why?

Security — your JWTSecret and DB password stay out of Git.
Flexibility — the same compiled binary runs on your laptop, a test server, and production, just by changing environment variables. No code edits, no rebuild.

Here's the flow: godotenv.Load() reads a local .env file (during development) and loads those values into the environment. Then each getEnv(...) call looks up a variable, and if it's missing, falls back to a sensible default. All of it runs once at startup because var Envs = initConfig() executes when the package is first imported. From then on, anywhere in your code you just write configs.Envs.DBName and get the value.
The getEnvAsInt helper exists because environment variables are always strings — so to get a number (like the JWT expiration), you must parse it with strconv.ParseInt, and fall back to the default if parsing fails.
One security note worth internalizing: the defaults here ("mypassword", "not-so-secret-now-is-it?") are fine for local learning, but in production those must come from real environment variables, never the fallbacks. The joke default secret is intentional — it's a reminder to override it.












*/













/*

What "type of file" this is
This is a config / settings file (often named config.go, env.go, or placed in a configs/ package). Almost every real backend has one. Its defining traits:
It sits at the foundation of the app — most other packages import it, but it imports almost nothing of your own code. It runs its setup once at startup. And it acts as the single boundary between the outside world (environment, .env, deployment platform) and your typed Go code.
The one thing you must add alongside it: a .env file in your project root (with the real values) and a matching entry in .gitignore so .env never gets committed. A common courtesy is to also commit a .env.example with the keys but fake values, so teammates know what variables they need to set.
Want me to sketch what your .env and .env.example should look like for this exact config, or move on to whatever file comes next?












*/