// Package hc represents logic of making the health check.
package hc

import (
	"context"
	"maps"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Compilation time checks for interface implementation.
var (
	_ HealthChecker = (*MultiChecker)(nil)
	_ HealthChecker = NopChecker{}
	_ HealthChecker = (*MultiServiceChecker)(nil)
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
	g, gctx := errgroup.WithContext(ctx)

	for _, check := range c.hcs {
		g.Go(func() error { return check.Health(gctx) })
	}

	return g.Wait()
}

// Add appends health HealthChecker to internal slice of HealthCheckers.
func (c *MultiChecker) Add(hc HealthChecker) { c.hcs = append(c.hcs, hc) }

// ServiceStatus represents the status of a service health check.
type ServiceStatus struct {
	Error     error
	Duration  time.Duration
	CheckedAt time.Time
}

// ServiceReport contains the status of all services.
type ServiceReport struct {
	mu sync.RWMutex
	st map[string]ServiceStatus
}

// NewServiceReport creates a new ServiceReport.
func NewServiceReport() *ServiceReport { return &ServiceReport{st: make(map[string]ServiceStatus)} }

// GetStatuses returns a copy of all service statuses.
func (r *ServiceReport) GetStatuses() map[string]ServiceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return maps.Clone(r.st)
}

// MultiServiceChecker implements the HealthChecker interface for checking multiple services.
type MultiServiceChecker struct {
	services map[string]HealthChecker
	report   *ServiceReport
}

// NewMultiServiceChecker creates a new MultiServiceChecker with the given services.
func NewMultiServiceChecker(report *ServiceReport) *MultiServiceChecker {
	return &MultiServiceChecker{
		services: make(map[string]HealthChecker),
		report:   report,
	}
}

// Report returns a service report.
func (c *MultiServiceChecker) Report() *ServiceReport {
	if c.report == nil {
		c.report = NewServiceReport()
	}

	return c.report
}

// AddService adds a service to be checked.
func (c *MultiServiceChecker) AddService(name string, checker HealthChecker) {
	c.services[name] = checker
}

// Health implements the HealthChecker interface.
func (c *MultiServiceChecker) Health(ctx context.Context) error {
	if len(c.services) == 0 {
		return nil
	}

	var g errgroup.Group

	for name, checker := range c.services {
		g.Go(func() error {
			startTime := time.Now()
			checkErr := checker.Health(ctx)
			duration := time.Since(startTime)

			c.report.mu.Lock()
			defer c.report.mu.Unlock()

			c.report.st[name] = ServiceStatus{
				Error:     checkErr,
				Duration:  duration,
				CheckedAt: time.Now(),
			}

			return checkErr
		})
	}

	return g.Wait()
}

// NopChecker represents nop health checker.
type NopChecker struct{}

// NewNopChecker returns new instance of NopChecker.
func NewNopChecker() NopChecker { return NopChecker{} }

func (NopChecker) Health(context.Context) error { return nil }

// Synchronizer holds synchronization mechanics.
// Deprecated: Use errgroup.Group instead.
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
