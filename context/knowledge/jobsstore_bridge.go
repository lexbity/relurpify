package knowledge

import (
	"codeburg.org/lexbit/relurpify/context/jobsstore"
	"codeburg.org/lexbit/relurpify/jobs"
)

// jobsStoreOpenInMemory opens the ephemeral jobs store used for handler
// checkpoint writes in tests.
func jobsStoreOpenInMemory() (jobs.Store, error) {
	return jobsstore.Open(jobsstore.Options{InMemory: true})
}
