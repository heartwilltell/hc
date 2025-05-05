package hc

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestMultiChecker_Health(t *testing.T) {
	errTest1 := errors.New("test error 1")
	errTest2 := errors.New("test error 2")

	type tcase struct {
		checkers []HealthChecker
		want     error
	}

	tests := map[string]tcase{
		"Nil checkers": {
			checkers: nil,
			want:     nil,
		},

		"Nil error": {
			checkers: []HealthChecker{
				&testChecker{HealthFunc: func(ctx context.Context) error { return nil }},
				&testChecker{HealthFunc: func(ctx context.Context) error { return nil }},
			},
			want: nil,
		},

		"One non nil error": {
			checkers: []HealthChecker{
				&testChecker{HealthFunc: func(ctx context.Context) error { return errTest1 }},
				&testChecker{HealthFunc: func(ctx context.Context) error { return nil }},
			},
			want: errTest1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := NewMultiChecker(tc.checkers...)
			if err := c.Health(context.Background()); err != tc.want {
				t.Errorf("Health() error = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("Multiple non nil errors", func(t *testing.T) {
		checkers := []HealthChecker{
			&testChecker{HealthFunc: func(ctx context.Context) error { return errTest1 }},
			&testChecker{HealthFunc: func(ctx context.Context) error { return errTest2 }},
		}

		err := NewMultiChecker(checkers...).Health(context.Background())
		if err == nil {
			t.Errorf("Health() error = nil, want non-nil (either %v or %v)", errTest1, errTest2)
		}
		if !errors.Is(err, errTest1) && !errors.Is(err, errTest2) {
			t.Errorf("Health() error = %v, want %v or %v", err, errTest1, errTest2)
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		checker := &testChecker{HealthFunc: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		}}
		mc := NewMultiChecker(checker)

		cancel()

		if err := mc.Health(ctx); err != context.Canceled {
			t.Errorf("Health() error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestMultiChecker_Add(t *testing.T) {
	tc1 := &testChecker{HealthFunc: func(ctx context.Context) error { return nil }}
	tc2 := &testChecker{HealthFunc: func(ctx context.Context) error { return errors.New("err2") }}

	mc1 := NewMultiChecker(tc1)
	if len(mc1.hcs) != 1 {
		t.Errorf("Initial add: expected len = 1, got = %d", len(mc1.hcs))
	}
	if !reflect.DeepEqual(mc1.hcs[0], tc1) {
		t.Errorf("Initial add: expected = %v, got = %v", tc1, mc1.hcs[0])
	}

	mc2 := NewMultiChecker(tc1, tc2)

	if len(mc2.hcs) != 2 {
		t.Fatalf("Add after init: expected len = 2, got = %d", len(mc2.hcs))
	}
	if !reflect.DeepEqual(mc2.hcs[0], tc1) {
		t.Errorf("Add after init [0]: expected = %v, got = %v", tc1, mc2.hcs[0])
	}
	if !reflect.DeepEqual(mc2.hcs[1], tc2) {
		t.Errorf("Add after init [1]: expected = %v, got = %v", tc2, mc2.hcs[1])
	}
}

type testChecker struct {
	HealthFunc func(ctx context.Context) error
}

func (c *testChecker) Health(ctx context.Context) error {
	if c.HealthFunc == nil {
		return nil
	}

	return c.HealthFunc(ctx)
}

func TestMultiServiceChecker_Health(t *testing.T) {
	errTest := errors.New("service failed")
	ctx := context.Background()

	report := NewServiceReport()
	checker := NewMultiServiceChecker(report)

	// Test with no services
	if err := checker.Health(ctx); err != nil {
		t.Errorf("Health() with no services should return nil, got %v", err)
	}
	if len(report.GetStatuses()) != 0 {
		t.Errorf("Report should be empty when no services are checked, got %d statuses", len(report.GetStatuses()))
	}

	checker.AddService("ok_service", &testChecker{HealthFunc: func(ctx context.Context) error { return nil }})
	checker.AddService("fail_service", &testChecker{HealthFunc: func(ctx context.Context) error { return errTest }})
	checker.AddService("slow_service", &testChecker{HealthFunc: func(ctx context.Context) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}})

	err := checker.Health(ctx)
	if err == nil {
		t.Errorf("Health() error = nil, want non-nil (expected error from fail_service)")
	}
	if !errors.Is(err, errTest) {
		t.Errorf("Health() error = %v, want %v", err, errTest)
	}

	statuses := report.GetStatuses()
	if len(statuses) != 3 {
		t.Fatalf("Expected 3 statuses in report, got %d", len(statuses))
	}

	if status, ok := statuses["ok_service"]; !ok {
		t.Errorf("Status for 'ok_service' not found")
	} else {
		if status.Error != nil {
			t.Errorf("Expected 'ok_service' error to be nil, got %v", status.Error)
		}
		if status.CheckedAt.IsZero() {
			t.Errorf("'ok_service' CheckedAt should not be zero")
		}
		
		if status.Duration < 0 {
			t.Errorf("'ok_service' Duration should be non-negative, got %v", status.Duration)
		}
	}

	if status, ok := statuses["fail_service"]; !ok {
		t.Errorf("Status for 'fail_service' not found")
	} else {
		if status.Error != errTest {
			t.Errorf("Expected 'fail_service' error to be %v, got %v", errTest, status.Error)
		}
		if status.CheckedAt.IsZero() {
			t.Errorf("'fail_service' CheckedAt should not be zero")
		}
		if status.Duration < 0 {
			t.Errorf("'fail_service' Duration should be non-negative, got %v", status.Duration)
		}
	}

	if status, ok := statuses["slow_service"]; !ok {
		t.Errorf("Status for 'slow_service' not found")
	} else {
		if status.Error != nil {
			t.Errorf("Expected 'slow_service' error to be nil, got %v", status.Error)
		}
		if status.CheckedAt.IsZero() {
			t.Errorf("'slow_service' CheckedAt should not be zero")
		}
		if status.Duration < 50*time.Millisecond {
			t.Errorf("'slow_service' Duration should be at least 50ms, got %v", status.Duration)
		}
	}

	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()

	checkerCancel := NewMultiServiceChecker(NewServiceReport())
	checkerCancel.AddService("cancel_service", &testChecker{HealthFunc: func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return errors.New("should have been cancelled")
		}
	}})

	if err := checkerCancel.Health(ctxCancel); err != context.Canceled {
		t.Errorf("Health() with cancelled context error = %v, want %v", err, context.Canceled)
	}

	cancelStatuses := checkerCancel.Report().GetStatuses()
	if status, ok := cancelStatuses["cancel_service"]; !ok {
		t.Errorf("Status for 'cancel_service' not found")
	} else if status.Error != context.Canceled {
		t.Errorf("Expected 'cancel_service' error to be context.Canceled, got %v", status.Error)
	}
}

func TestMultiServiceChecker_Report(t *testing.T) {
	providedReport := NewServiceReport()
	checkerWithReport := NewMultiServiceChecker(providedReport)
	if checkerWithReport.Report() != providedReport {
		t.Error("Report() should return the provided report instance")
	}

	checkerNilReport := NewMultiServiceChecker(nil)
	report := checkerNilReport.Report()
	if report == nil {
		t.Fatal("Report() should create a new report if initialized with nil")
	}
	if len(report.st) != 0 {
		t.Error("Newly created report should be empty")
	}
	if checkerNilReport.Report() != report {
		t.Error("Subsequent calls to Report() should return the same created instance")
	}
}

func TestMultiServiceChecker_AddService(t *testing.T) {
	checker := NewMultiServiceChecker(nil)
	svc1 := &testChecker{}
	svc2 := &testChecker{}

	checker.AddService("service1", svc1)
	checker.AddService("service2", svc2)

	if len(checker.services) != 2 {
		t.Fatalf("Expected 2 services, got %d", len(checker.services))
	}
	if checker.services["service1"] != svc1 {
		t.Errorf("Mismatch for service1")
	}
	if checker.services["service2"] != svc2 {
		t.Errorf("Mismatch for service2")
	}
}

func TestServiceReport_GetStatuses(t *testing.T) {
	report := NewServiceReport()
	report.st["service1"] = ServiceStatus{Error: nil, Duration: 1 * time.Second, CheckedAt: time.Now()}

	statuses := report.GetStatuses()

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 status, got %d", len(statuses))
	}
	if _, ok := statuses["service1"]; !ok {
		t.Fatal("Expected status for 'service1'")
	}

	statuses["service2"] = ServiceStatus{Error: errors.New("new error")}
	if len(report.st) != 1 {
		t.Error("Original report map should not be modified after modifying the copy")
	}
	if _, ok := report.st["service2"]; ok {
		t.Error("Original report map should not contain 'service2'")
	}
}

func TestNopChecker_Health(t *testing.T) {
	checker := NewNopChecker()
	if err := checker.Health(context.Background()); err != nil {
		t.Errorf("NopChecker.Health() error = %v, want nil", err)
	}
}
