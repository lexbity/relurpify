// Package jobs defines the framework-owned job boundary.
//
// A task is user intent or a work request. A job is the executable unit created
// from that intent, carrying lifecycle state and orchestration policy. Workers
// execute jobs. Agents are one kind of worker, but the jobs package does not
// own agent taxonomy or capability policy. Capability selection may influence
// worker routing, but it remains outside this package's responsibility.
package jobs
