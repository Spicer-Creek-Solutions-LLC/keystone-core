//go:build race

package nats

func raceEnabled() bool {
	return true
}
