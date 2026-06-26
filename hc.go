// Package hc represents logic of making the health check.
package hc

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync"
	"time"
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

// ErrNilChecker is returned when a nil HealthChecker is registered.
var ErrNilChecker = errors.New("hc: nil health checker")

// NewMultiChecker takes several health checkers and performs
// health checks for each of them concurrently.
func NewMultiChecker(hcs ...HealthChecker) *MultiChecker {
	c := MultiChecker{hcs: make([]HealthChecker, 0, len(hcs))}
	c.hcs = append(c.hcs, hcs...)
	return &c
}

// MultiChecker takes multiple health checker and performs
// health checks for each of them concurrently.
type MultiChecker struct {
	mu  sync.RWMutex
	hcs []HealthChecker
}

// Health takes the context and performs the health check.
// Returns nil in case of success or an error in case
// of a failure.
func (c *MultiChecker) Health(ctx context.Context) error {
	hcs := c.checkers()
	if len(hcs) == 0 {
		return nil
	}

	checkCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstErr firstError
	var wg sync.WaitGroup
	for _, check := range hcs {
		check := check
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := runHealthCheck(checkCtx, check); err != nil {
				firstErr.set(err)
				cancel()
			}
		}()
	}

	wg.Wait()
	return firstErr.err
}

// Add appends health HealthChecker to internal slice of HealthCheckers.
func (c *MultiChecker) Add(hc HealthChecker) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hcs = append(c.hcs, hc)
}

func (c *MultiChecker) checkers() []HealthChecker {
	if c == nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return append([]HealthChecker(nil), c.hcs...)
}

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
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return maps.Clone(r.st)
}

func (r *ServiceReport) setStatus(name string, status ServiceStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.st == nil {
		r.st = make(map[string]ServiceStatus)
	}

	r.st[name] = status
}

// MultiServiceChecker implements the HealthChecker interface for checking multiple services.
type MultiServiceChecker struct {
	mu       sync.RWMutex
	services map[string]HealthChecker
	report   *ServiceReport
}

// NewMultiServiceChecker creates a new MultiServiceChecker with the given services.
func NewMultiServiceChecker(report *ServiceReport) *MultiServiceChecker {
	serviceReport := report
	if serviceReport == nil {
		serviceReport = NewServiceReport()
	}

	return &MultiServiceChecker{
		services: make(map[string]HealthChecker),
		report:   serviceReport,
	}
}

// Report returns a service report.
func (c *MultiServiceChecker) Report() *ServiceReport {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.report == nil {
		c.report = NewServiceReport()
	}

	return c.report
}

// AddService adds a service to be checked.
func (c *MultiServiceChecker) AddService(name string, checker HealthChecker) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.services == nil {
		c.services = make(map[string]HealthChecker)
	}

	c.services[name] = checker
}

// Health implements the HealthChecker interface.
func (c *MultiServiceChecker) Health(ctx context.Context) error {
	services, report := c.snapshot()
	if len(services) == 0 {
		return nil
	}

	var firstErr firstError
	var wg sync.WaitGroup
	for name, checker := range services {
		name, checker := name, checker
		wg.Add(1)
		go func() {
			defer wg.Done()

			startTime := time.Now()
			checkErr := runHealthCheck(ctx, checker)
			checkedAt := time.Now()

			report.setStatus(name, ServiceStatus{
				Error:     checkErr,
				Duration:  checkedAt.Sub(startTime),
				CheckedAt: checkedAt,
			})

			if checkErr != nil {
				firstErr.set(checkErr)
			}
		}()
	}

	wg.Wait()
	return firstErr.err
}

func (c *MultiServiceChecker) snapshot() (map[string]HealthChecker, *ServiceReport) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.report == nil {
		c.report = NewServiceReport()
	}
	if len(c.services) == 0 {
		return nil, c.report
	}

	services := maps.Clone(c.services)
	return services, c.report
}

// NopChecker represents nop health checker.
type NopChecker struct{}

// NewNopChecker returns new instance of NopChecker.
func NewNopChecker() NopChecker { return NopChecker{} }

func (NopChecker) Health(context.Context) error { return nil }

// Synchronizer holds synchronization mechanics.
//
// Deprecated: Use sync.WaitGroup or another coordination primitive instead.
type Synchronizer struct {
	wg     sync.WaitGroup
	so     sync.Once
	err    error
	cancel func()
}

// Error returns a string representation of underlying error.
// Implements builtin error interface.
func (s *Synchronizer) Error() string {
	if s == nil || s.err == nil {
		return ""
	}

	return s.err.Error()
}

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
func (s *Synchronizer) Cancel() {
	if s == nil || s.cancel == nil {
		return
	}

	s.cancel()
}

type firstError struct {
	once sync.Once
	err  error
}

func (e *firstError) set(err error) {
	if err == nil {
		return
	}

	e.once.Do(func() { e.err = err })
}

func runHealthCheck(ctx context.Context, checker HealthChecker) error {
	if isNilHealthChecker(checker) {
		return ErrNilChecker
	}

	return checker.Health(ctx)
}

func isNilHealthChecker(checker HealthChecker) bool {
	if checker == nil {
		return true
	}

	value := reflect.ValueOf(checker)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
