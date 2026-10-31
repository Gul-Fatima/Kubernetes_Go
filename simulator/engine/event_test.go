package engine

import (
	"testing"
	"time"
)

func TestEventQueueChronologicalOrder(t *testing.T) {
	scheduler := NewEventScheduler()

	events := []Event{
		{Time: 30 * time.Minute, Type: EventWorkloadCompletion},
		{Time: 10 * time.Minute, Type: EventWorkloadArrival},
		{Time: 20 * time.Minute, Type: EventWorkloadArrival},
	}

	for _, event := range events {
		if err := scheduler.Schedule(event); err != nil {
			t.Fatal(err)
		}
	}

	expected := []time.Duration{
		10 * time.Minute,
		20 * time.Minute,
		30 * time.Minute,
	}

	for _, want := range expected {
		got := scheduler.Next()
		if got == nil {
			t.Fatal("unexpected empty event queue")
		}
		if got.Time != want {
			t.Errorf("expected event at %v, got %v", want, got.Time)
		}
	}

	if scheduler.Len() != 0 {
		t.Errorf("expected empty queue, got %d events", scheduler.Len())
	}
}

func TestEventQueueSameTimestamp(t *testing.T) {
	scheduler := NewEventScheduler()

	events := []EventType{
		EventWorkloadArrival,
		EventSimulationEnd,
		EventWorkloadCompletion,
	}

	for _, eventType := range events {
		if err := scheduler.Schedule(Event{
			Time: time.Minute,
			Type: eventType,
		}); err != nil {
			t.Fatal(err)
		}
	}

	expected := []EventType{
		EventWorkloadCompletion,
		EventWorkloadArrival,
		EventSimulationEnd,
	}

	for _, want := range expected {
		got := scheduler.Next()
		if got == nil {
			t.Fatal("unexpected empty event queue")
		}
		if got.Type != want {
			t.Errorf("expected %s, got %s", want, got.Type)
		}
	}
}

func TestEventQueueRejectsInvalidEvents(t *testing.T) {
	scheduler := NewEventScheduler()

	if err := scheduler.Schedule(Event{
		Time: -time.Second,
		Type: EventWorkloadArrival,
	}); err == nil {
		t.Error("expected error for negative event time")
	}

	if err := scheduler.Schedule(Event{
		Time: time.Second,
	}); err == nil {
		t.Error("expected error for empty event type")
	}
}
