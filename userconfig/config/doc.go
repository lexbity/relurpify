/*
Package config centralizes project configuration loading and validation.

This is the only package that:

- Reads configuration schema in relurpify_cfg/**
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
package config
