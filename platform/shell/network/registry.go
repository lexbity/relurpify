package network

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/platform/shell/catalog"
)

// Tools returns networking helpers.
func Tools(basePath string) []core.Tool {
	return []core.Tool{
		NewCurlTool(basePath),
		NewWgetTool(basePath),
		NewNCTool(basePath),
		NewDigTool(basePath),
		NewNslookupTool(basePath),
		NewIPTool(basePath),
		NewSSTool(basePath),
		NewPingTool(basePath),
	}
}

// CatalogEntries returns declarative catalog metadata for the network family.
func CatalogEntries() []catalog.ToolCatalogEntry {
	specs := []catalog.CommandToolSpec{
		{Name: "cli_curl", Family: "network", Intent: []string{"fetch", "http"}, Description: "Transfers data over HTTP(S) using curl.", Command: "curl", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_wget", Family: "network", Intent: []string{"fetch", "http"}, Description: "Downloads resources with wget.", Command: "wget", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_nc", Family: "network", Intent: []string{"probe", "tcp"}, Description: "Creates TCP/UDP connections via netcat (nc).", Command: "nc", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_dig", Family: "network", Intent: []string{"dns", "probe"}, Description: "Queries DNS records using dig.", Command: "dig", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_nslookup", Family: "network", Intent: []string{"dns", "probe"}, Description: "Performs DNS lookups via nslookup.", Command: "nslookup", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_ip", Family: "network", Intent: []string{"inspect", "routing"}, Description: "Inspects network interfaces with ip.", Command: "ip", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_ss", Family: "network", Intent: []string{"inspect", "sockets"}, Description: "Inspects sockets using ss.", Command: "ss", Tags: []string{core.TagExecute, core.TagNetwork}},
		{Name: "cli_ping", Family: "network", Intent: []string{"probe", "icmp"}, Description: "Checks host reachability with ping.", Command: "ping", Tags: []string{core.TagExecute, core.TagNetwork}},
	}
	entries := make([]catalog.ToolCatalogEntry, 0, len(specs))
	for _, spec := range specs {
		entries = append(entries, catalog.EntryFromCommandSpec(spec))
	}
	return entries
}
