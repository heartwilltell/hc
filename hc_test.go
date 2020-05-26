package hc

import (
	"context"
	"errors"
	"testing"
)

func TestMultiChecker_Health(t *testing.T) {
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
		"Non nil error": {
			checkers: []HealthChecker{
				&testChecker{HealthFunc: func(ctx context.Context) error { return errTest }},
				&testChecker{HealthFunc: func(ctx context.Context) error { return nil }},
			},
			want: errTest,
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
}

var errTest = errors.New("test error")

type testChecker struct {
	HealthFunc func(ctx context.Context) error
}

func (c *testChecker) Health(ctx context.Context) error {
	if c.HealthFunc == nil {
		return nil
	}
	return c.HealthFunc(ctx)
}
