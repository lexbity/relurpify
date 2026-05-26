/*
	cfgload - configuration

# Centralizies all project configuration and loading of contfiguration

This only package that:

- Reads all schema in relurpify_cfg/**
- Reads env / CLI override inputs at the boundary
- Parses schema declarations
- Validates cross-file config references
- Builds the resolved AppConfig
- Builds all loaded registries:
  - model provider registry
  - model profile registry
  - tool (capability) registry
  - agent registry
  - security bundle
*/
package cfgload
