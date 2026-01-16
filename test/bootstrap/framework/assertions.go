package framework

import "fmt"

// Assertion describes a bootstrap verification step.
type Assertion struct {
	Name string
	Run  func() error
}

// Assert returns a failure if condition is false.
func Assert(name string, ok bool, message string) *Assertion {
	return &Assertion{
		Name: name,
		Run: func() error {
			if ok {
				return nil
			}
			return fmt.Errorf("%s: %s", name, message)
		},
	}
}
