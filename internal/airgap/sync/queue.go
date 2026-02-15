package sync

import "container/heap"

// operationQueue implements heap.Interface for OperationConfig, ordered by priority.
type operationQueue []OperationConfig

func (q operationQueue) Len() int { return len(q) }

func (q operationQueue) Less(i, j int) bool {
	return q[i].Priority < q[j].Priority
}

func (q operationQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *operationQueue) Push(x interface{}) {
	*q = append(*q, x.(OperationConfig))
}

func (q *operationQueue) Pop() interface{} {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

// sortedOperations returns operations sorted by priority (lowest value first).
func sortedOperations(ops []OperationConfig) []OperationConfig {
	q := make(operationQueue, len(ops))
	copy(q, ops)
	heap.Init(&q)

	sorted := make([]OperationConfig, 0, len(ops))
	for q.Len() > 0 {
		sorted = append(sorted, heap.Pop(&q).(OperationConfig))
	}
	return sorted
}
