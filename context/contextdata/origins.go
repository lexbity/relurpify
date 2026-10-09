package contextdata

// OriginClass identifies the dataflow origin of an envelope value. The
// envelope records it at capture time and the grounding service reads it to
// compute the trust floor. Restrictiveness, ascending: user < tool < llm —
// an llm-origin value is the least provable, a user-origin value the most.
type OriginClass string

const (
	OriginUser OriginClass = "user"
	OriginTool OriginClass = "tool"
	OriginLLM  OriginClass = "llm"
)

// Valid reports whether the class is a member of the closed set.
func (o OriginClass) Valid() bool {
	switch o {
	case OriginUser, OriginTool, OriginLLM:
		return true
	default:
		return false
	}
}

// MostRestrictive returns the more restrictive of two origin classes (llm >
// tool > user). It is the merge lattice the envelope uses when combining
// values from multiple sources.
func MostRestrictive(a, b OriginClass) OriginClass {
	if originRestrictiveness(a) >= originRestrictiveness(b) {
		return a
	}
	return b
}

func originRestrictiveness(o OriginClass) int {
	switch o {
	case OriginUser:
		return 1
	case OriginTool:
		return 2
	case OriginLLM:
		return 3
	default:
		return 3
	}
}
