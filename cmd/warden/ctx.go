package main

import (
	"context"
	"time"
)

// withTimeout runs f with a context that ends after d.
func withTimeout[T any](d time.Duration, f func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return f(ctx)
}
