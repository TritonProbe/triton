package buildinfo

var (
	Version   = "dev"
	BuildTime = "unknown"
)

func Set(version, buildTime string) {
	if version != "" {
		Version = version
	}
	if buildTime != "" {
		BuildTime = buildTime
	}
}
