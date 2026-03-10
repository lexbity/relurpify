package core

import "time"

func parsePolicyDuration(raw string) (time.Duration, error) {
	return time.ParseDuration(raw)
}
