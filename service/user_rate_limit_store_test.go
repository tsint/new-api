package service

import (
	"testing"
	"time"
)

func TestMemUserRateLimitStoreConcurrency(t *testing.T) {
	s := NewUserRateLimitStore(nil)
	if s == nil {
		t.Fatal("nil redis must yield in-memory store")
	}

	if got, err := s.AcquireConcurrency("u1"); err != nil || got != 1 {
		t.Fatalf("first acquire = %d, %v, want 1", got, err)
	}
	if got, _ := s.AcquireConcurrency("u1"); got != 2 {
		t.Fatalf("second acquire = %d, want 2", got)
	}
	s.ReleaseConcurrency("u1")
	if got, _ := s.AcquireConcurrency("u1"); got != 2 {
		t.Fatalf("acquire after release = %d, want 2", got)
	}

	// 独立 key 互不影响
	if got, _ := s.AcquireConcurrency("u2"); got != 1 {
		t.Fatalf("other key acquire = %d, want 1", got)
	}

	// 释放不能低于0
	for i := 0; i < 5; i++ {
		s.ReleaseConcurrency("u3")
	}
	s.AcquireConcurrency("u3")
	if got := s.GetConcurrency("u3"); got != 1 {
		t.Fatalf("after over-release and acquire, u3 = %d, want 1", got)
	}
}

func TestMemUserRateLimitStoreSecondWindow(t *testing.T) {
	s := NewUserRateLimitStore(nil)
	base := time.Unix(1700000000, 0)

	cases := []struct {
		at   time.Time
		want int64
	}{
		{base, 1},
		{base.Add(300 * time.Millisecond), 2},
		{base.Add(700 * time.Millisecond), 3},
		// 同一自然秒(毫秒尾部)仍属同窗
		{base.Add(999 * time.Millisecond), 4},
		// 下一个自然秒全新窗口
		{base.Add(1 * time.Second), 1},
		{base.Add(1500 * time.Millisecond), 2},
	}
	for _, tc := range cases {
		got, err := s.IncSecondRate("u1", tc.at)
		if err != nil {
			t.Fatalf("inc at %v error: %v", tc.at, err)
		}
		if got != tc.want {
			t.Fatalf("inc at %v = %d, want %d", tc.at.Format(time.RFC3339Nano), got, tc.want)
		}
	}

	// key 相互独立
	if got, _ := s.IncSecondRate("u2", base.Add(1200*time.Millisecond)); got != 1 {
		t.Fatalf("independent key = %d, want 1", got)
	}
}
