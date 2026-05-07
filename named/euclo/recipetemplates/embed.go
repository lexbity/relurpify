package recipetemplates

import "embed"

//go:embed *.yaml intent/*.yaml
var templateFS embed.FS
