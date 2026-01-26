package identity

import (
	"testing"
	"time"
)

func TestWaitForRetryOrStop_StopsEarly(t *testing.T) {
	stopCh := make(chan struct{})
	done := make(chan bool, 1)

	go func() {
		done <- waitForRetryOrStop(stopCh, time.Second)
	}()

	close(stopCh)

	select {
	case ok := <-done:
		if ok {
			t.Fatal("expected stop to return false")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected wait to return promptly")
	}
}

func TestWaitForRetryOrStop_Elapsed(t *testing.T) {
	stopCh := make(chan struct{})
	done := make(chan bool, 1)

	go func() {
		done <- waitForRetryOrStop(stopCh, 10*time.Millisecond)
	}()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("expected timer to elapse")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected wait to complete")
	}
}
