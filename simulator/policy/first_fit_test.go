package policy

import (
	"testing"

	"eawsk/simulator/models"
)

func TestFirstFitPreservesCandidateOrder(t *testing.T) {
	policy := &FirstFitPolicy{}

	workload := &models.Workload{
		ID: "workload-1",
	}

	candidates := []*models.Server{
		{ID: "server-a"},
		{ID: "server-b"},
		{ID: "server-c"},
	}

	ranked, err := policy.Rank(workload, candidates)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(ranked))
	}

	for i, expected := range []string{"server-a", "server-b", "server-c"} {
		if ranked[i].ServerID != expected {
			t.Errorf("position %d: expected %s, got %s",
				i, expected, ranked[i].ServerID)
		}
	}
}

func TestFirstFitRejectsNilWorkload(t *testing.T) {
	policy := &FirstFitPolicy{}

	_, err := policy.Rank(nil, []*models.Server{{ID: "server-a"}})

	if err == nil {
		t.Fatal("expected error for nil workload")
	}
}
