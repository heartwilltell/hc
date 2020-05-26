# 👩‍⚕️👨‍⚕️ hc
`hc` is a tiny library for synchronization of mission critical concurrent health checks

The `HealthChecker` interface is a heart of this small library.
```go
// HealthChecker represents logic of making the health check.
type HealthChecker interface {
	// Health takes the context and performs the health check.
	// Returns nil in case of success or an error in case
	// of a failure.
	Health(ctx context.Context) error
}
``` 