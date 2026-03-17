// Package checkpoint provides resumability and recovery for chainer execution.
//
// ChainerAgent can checkpoint after each completed stage, allowing interrupted
// workflows to resume from the last completed stage rather than restarting.
//
// # Checkpoint Storage
//
// CheckpointStore persists checkpoint snapshots keyed by task ID + stage index.
// It wraps framework/pipeline.CheckpointStore for SQLite-backed persistence.
//
// # Recovery Manager
//
// RecoveryManager queries the checkpoint store to resume workflows:
//   - Finds last completed stage for a given task
//   - Returns resume checkpoint (context, stage index, prior results)
//   - Skips already-completed stages when re-executing
//
// # Usage Pattern
//
// Create a checkpoint store:
//
//   store := checkpoint.NewSQLiteStore(dbPath)
//
// Enable checkpointing in ChainerAgent:
//
//   agent.CheckpointStore = store
//   agent.CheckpointAfterStage = true
//
// On interruption, recovering is automatic:
//
//   manager := checkpoint.NewRecoveryManager(store)
//   resumeCP, err := manager.FindLastCheckpoint(taskID)
//   if resumeCP != nil {
//       // Pipeline runner will resume from resumeCP
//   }
package checkpoint
