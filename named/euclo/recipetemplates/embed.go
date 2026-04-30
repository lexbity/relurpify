package recipetemplates

import "embed"

//go:embed *.yaml
var templateFS embed.FS
