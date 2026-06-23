// Package ingestfixture provides the ingestion node as a test fixture.
//
// The ingestion node populates the knowledge store before context streaming.
// Its production wiring (the thoughtrecipes "ingest" DSL) was removed, so the
// node is retained here purely to seed the knowledge store in euclo tests.
package ingestfixture
