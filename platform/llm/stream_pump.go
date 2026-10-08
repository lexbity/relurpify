package llm

import (
	"context"
	"sync/atomic"
)

// pumpStream is the canonical token pump for LanguageModel stream
// implementations (contract R1-R6 in model/ports.go). It forwards tokens from
// src to out until src closes or ctx is cancelled, then closes out exactly
// once. Tokens dropped because ctx was already done increment drops.
func pumpStream(ctx context.Context, src <-chan string, out chan string, drops *atomic.Int64) {
	defer close(out)
	for {
		select {
		case tok, ok := <-src:
			if !ok {
				return
			}
			select {
			case out <- tok:
			case <-ctx.Done():
				drops.Add(1)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
