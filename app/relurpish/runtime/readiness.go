package runtime

// Readiness carries the workspace's degradation state. Canonical readiness is
// doctor's (BuildDoctorReport -> DoctorReport.Ready); status and the TUI gate
// consume that verdict, so this struct only records when a composed runtime
// fell back to the degraded deny-all path and why.
type Readiness struct {
	Degraded bool
	Reason   string
}
