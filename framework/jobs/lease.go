package jobs

import "context"

type LeaseManager interface {
	RenewLease(ctx context.Context, jobID, leaseID string) (*JobLease, error)
	ReleaseLease(ctx context.Context, jobID, leaseID, reason string) (*JobLease, error)
	ExpireLease(ctx context.Context, jobID, leaseID string) (*JobLease, error)
}
