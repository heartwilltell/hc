// Package hc represents logic of making the health check.
package hc

import (
	"context"
	"sync"
)

// Compilation time checks for interface implementation.
var (
	_ HealthChecker = (*MultiChecker)(nil)
	_ HealthChecker = NopChecker{}
)

// HealthChecker represents logic of making the health check.
type HealthChecker interface {
	// Health takes the context and performs the health check.
	// Returns nil in case of success or an error in case
	// of a failure.
	Health(ctx context.Context) error
}

// NewMultiChecker takes several health checkers and performs
// health checks for each of them concurrently.
func NewMultiChecker(hcs ...HealthChecker) *MultiChecker {
	c := MultiChecker{hcs: make([]HealthChecker, 0, len(hcs))}
	c.hcs = append(c.hcs, hcs...)
	return &c
}

// MultiChecker takes multiple health checker and performs
// health checks for each of them concurrently.
type MultiChecker struct{ hcs []HealthChecker }

// Health takes the context and performs the health check.
// Returns nil in case of success or an error in case
// of a failure.
func (c *MultiChecker) Health(ctx context.Context) error {
	hctx, cancel := context.WithCancel(ctx)
	s := Synchronizer{cancel: cancel}

	for _, check := range c.hcs {
		s.Add(1)

		go func(ctx context.Context, s *Synchronizer, check func(ctx context.Context) error) {
			defer s.Done()
			select {
			case <-ctx.Done():
				s.SetError(ctx.Err())
			default:
				if err := check(ctx); err != nil {
					s.SetError(err)
					s.cancel()
				}
			}
		}(hctx, &s, check.Health)
	}

	s.Wait()
	return s.err
}

// Add appends health HealthChecker to internal slice of HealthCheckers.
func (c *MultiChecker) Add(hc HealthChecker) { c.hcs = append(c.hcs, hc) }

// NopChecker represents nop health checker.
type NopChecker struct{}

// NewNopChecker returns new instance of NopChecker.
func NewNopChecker() NopChecker { return NopChecker{} }

func (n NopChecker) Health(context.Context) error { return nil }

// Synchronizer holds synchronization mechanics.
type Synchronizer struct {
	wg     sync.WaitGroup
	so     sync.Once
	err    error
	cancel func()
}

// Error returns a string representation of underlying error.
// Implements builtin error interface.
func (s *Synchronizer) Error() string { return s.err.Error() }

// SetError sets an error to the Synchronizer structure.
// Uses sync.Once to set error only once.
func (s *Synchronizer) SetError(err error) { s.so.Do(func() { s.err = err }) }

// Do wrap the sync.Once Do method.
func (s *Synchronizer) Do(f func()) { s.so.Do(f) }

// Done wraps the sync.WaitGroup Done method.
func (s *Synchronizer) Done() { s.wg.Done() }

// Add wraps the sync.WaitGroup Add method.
func (s *Synchronizer) Add(delta int) { s.wg.Add(delta) }

// Wait wraps the sync.WaitGroup Wait method.
func (s *Synchronizer) Wait() { s.wg.Wait() }

// Cancel calls underlying cancel function to cancel context,
// which passed to all health checks function.
func (s *Synchronizer) Cancel() { s.cancel() }
