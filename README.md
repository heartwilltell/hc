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

## Usage

Let's say that we have a web application with some downstream services (database, remote storage etc.), 
Work of these services is critical for our application. So we need to check if they are reachable and healthy, 
to provide the overall service health check information to orchestrator or load balancer.

With `hc` it is simple. You just need to implement the `HealthChecker` interface for you're downstream.

```go
// PgDownstreamService holds logic of interaction 
// with Postgres database.
type PgDownstreamService struct {
    db *pgxpool.Pool
}

func (s *PgDownstreamService) Health(ctx context.Context) error {
    conn, err := s.db.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("unable to aquire connection from pool: %w", err)
    }
    defer conn.Release()

    q := `SELECT count(*) FROM information_schema.tables WHERE table_type='public';`

    var count int
    if err := conn.QueryRow(ctx, q).Scan(&count); err != nil {
        return fmt.Errorf("query failed: %w", err)
    }
    return nil
}
```

Now in your http server health check endpoint you just need to gather information about all downstream health checks.

```go
checker := hc.NewMultiChecker(pgDownstrean, storageDownstream)

mux := http.NewServeMux()
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    if err := checker.Health(r.Context()); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
})
``` 

