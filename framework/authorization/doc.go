// Package authorization enforces agent permission contracts at execution time
// using a three-level policy model: Allow, Ask, and Deny.
//
// # PermissionManager
//
// PermissionManager is the runtime gatekeeper. Before any tool invocation it
// evaluates the tool's required permissions (file paths, executables, network
// endpoints) against the active policy set derived from the agent manifest.
//
// # Policy engine
//
// PolicyEngine compiles declarative PolicyRules (defined in framework/core)
// into a fast match structure. policy_compile.go builds the compiled form;
// policy_match.go evaluates incoming capability requests against it.
//
// # Human-in-the-loop
//
// hitl.go implements the Ask path: when a requested action is not explicitly
// allowed or denied, execution pauses and a HITLRequest is surfaced to the
// operator. The operator may approve once ([y]), for the session ([s]),
// permanently ([a]), or deny ([n]).
//
// # Command authorization
//
// command_authorization.go authorizes executable invocations, checking the
// binary name and argument pattern against the manifest's allowed executables.
//
// # Delegations
//
// delegations.go manages delegation contracts — bounded grants of capability
// authority issued by one agent to another for a scoped task.
package authorization
