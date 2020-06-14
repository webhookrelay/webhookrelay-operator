package version

var (
	// Version number that is being run at the moment.  Version should use semver.
	Version = "dev"
	// Revision that was compiled. This will be filled in by the compiler.
	Revision string
	// BuildDate is when the binary was compiled.  This will be filled in by the
	// compiler.
	BuildDate string
)
