//go:build !linux

package sockprobe

// ConsumptionObservable is false on every platform without an instrument. The
// ordering case then fails as unmeasurable rather than passing unmeasured; see
// unread_linux.go.
func ConsumptionObservable() bool { return false }
