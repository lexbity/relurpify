package ayenitd

import (
	"codeburg.org/lexbit/relurpify/context/jobsstore"
	"codeburg.org/lexbit/relurpify/jobs"
)

// jobsStoreOpen opens the durable jobs store at the given directory. Badger's
// per-directory lock is the single-executor invariant: a second opener
// returns an error and the runner exits nonzero (FR-17).
func jobsStoreOpen(dir string) (jobs.Store, error) {
	return jobsstore.Open(jobsstore.Options{Dir: dir})
}
