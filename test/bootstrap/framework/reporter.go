package framework

import "time"

// TestResult captures a single bootstrap test result.
type TestResult struct {
	Scenario string
	Platform string
	Passed   bool
	Error    string
	Started  time.Time
	Ended    time.Time
}

// Reporter records test results.
type Reporter interface {
	Record(result TestResult) error
}

// MemoryReporter stores results in memory.
type MemoryReporter struct {
	Results []TestResult
}

// Record appends a result to the reporter.
func (m *MemoryReporter) Record(result TestResult) error {
	m.Results = append(m.Results, result)
	return nil
}
