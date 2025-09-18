package notifications

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/9ssi7/exponent"
)

const DefaultAsyncTimeout = 8 * time.Second

// PushSender is just an abstraction over any push sender,
// but here it's directly tied to the exponent SDK types.
type PushSender interface {
	Publish(ctx context.Context, msgs []*exponent.Message) ([]*exponent.MessageResponse, error)
	PublishSingle(ctx context.Context, msg *exponent.Message) ([]*exponent.MessageResponse, error)
}

// CallAsync = run my function asynchronously with a timeout and print error log
func CallAsync(fn func(ctx context.Context) error, operationName string) {
	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := fn(ctx); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("🚨 TIMEOUT: %s took too long (>5s)", operationName)
			} else {
				log.Printf("❌ ERROR: %s failed: %v", operationName, err)
			}
		} else {
			duration := time.Since(start)
			log.Printf("✅ SUCCESS: %s completed in %v", operationName, duration)

			// Warn if it's getting slow but didn't timeout
			if duration > 3*time.Second {
				log.Printf("⚠️  WARNING: %s is slow: %v", operationName, duration)
			}
		}
	}()
}

func dedupe(tokens []string) []string {
	// set a already seem tokens
	seen := map[string]struct{}{}

	//It sets out to length 0 but with the same backing array and capacity as tokens.
	out := tokens[:0]

	//scan input left to right
	for _, t := range tokens {
		if t == "" {
			continue
		} //skip empties

		if _, ok := seen[t]; ok {
			continue
		} // skip duplicates
		seen[t] = struct{}{} // mark as seen
		out = append(out, t)

	}
	//return the compact slice
	return out
}
