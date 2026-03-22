package version

// Set via -ldflags at build time. Defaults are for local development.
var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildTime = "unknown"
)
