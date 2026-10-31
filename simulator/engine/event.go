package engine

import (
	"container/heap"
	"fmt"
	"time"
)

// EventType identifies the type of simulation event.
type EventType string

const (
	EventWorkloadArrival    EventType = "workload_arrival"
	EventWorkloadCompletion EventType = "workload_completion"
	EventSimulationEnd      EventType = "simulation_end"
)

// Event represents something that happens at a specific simulation time.
type Event struct {
	Time       time.Duration
	Type       EventType
	WorkloadID string
	ServerID   string
	Sequence   uint64
}

// EventQueue stores events in chronological order.
type EventQueue []*Event

func (q EventQueue) Len() int {
	return len(q)
}

// Less prioritizes events at the same time:
// 1. Workload completion
// 2. Workload arrival
// 3. Simulation end
func (q EventQueue) Less(i, j int) bool {
	if q[i].Time != q[j].Time {
		return q[i].Time < q[j].Time
	}

	priority := func(eventType EventType) int {
		switch eventType {
		case EventWorkloadCompletion:
			return 0
		case EventWorkloadArrival:
			return 1
		case EventSimulationEnd:
			return 2
		default:
			return 3
		}
	}

	priorityI := priority(q[i].Type)
	priorityJ := priority(q[j].Type)

	if priorityI != priorityJ {
		return priorityI < priorityJ
	}

	return q[i].Sequence < q[j].Sequence
}

func (q EventQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (q *EventQueue) Push(value any) {
	event, ok := value.(*Event)
	if !ok || event == nil {
		panic("event queue only accepts non-nil *Event values")
	}
	*q = append(*q, event)
}

func (q *EventQueue) Pop() any {
	old := *q
	n := len(old)
	event := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return event
}

// EventScheduler manages the event priority queue.
type EventScheduler struct {
	queue    EventQueue
	sequence uint64
}

// NewEventScheduler creates an empty event scheduler.
func NewEventScheduler() *EventScheduler {
	s := &EventScheduler{}
	heap.Init(&s.queue)
	return s
}

// Schedule adds an event to the queue.
func (s *EventScheduler) Schedule(event Event) error {
	if event.Time < 0 {
		return fmt.Errorf("event time cannot be negative")
	}

	if event.Type == "" {
		return fmt.Errorf("event type cannot be empty")
	}

	s.sequence++
	event.Sequence = s.sequence

	heap.Push(&s.queue, &event)
	return nil
}

// Next returns and removes the earliest scheduled event.
// It returns nil when the queue is empty.
func (s *EventScheduler) Next() *Event {
	if s.queue.Len() == 0 {
		return nil
	}

	return heap.Pop(&s.queue).(*Event)
}

// Len returns the number of pending events.
func (s *EventScheduler) Len() int {
	return s.queue.Len()
}
