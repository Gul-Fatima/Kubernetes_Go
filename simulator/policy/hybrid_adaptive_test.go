package policy

import (
	"testing"
	"time"

	"eawsk/simulator/energy"
	"eawsk/simulator/models"
)

func TestHybridAdaptiveUsesTopKShortlist(t *testing.T) {
	policy := &HybridAdaptivePolicy{
		TopK: 2,
	}

	workload := &models.Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 1,
		},
	}

	candidates := []*models.Server{
		{
			ID: "server-a",
			Capacity: models.ResourceCapacity{
				CPUCores:    10,
				MemoryBytes: 100,
			},
			PowerProfile: energy.PowerProfile{
				IdlePowerW:       100,
				CPUDynamicPowerW: 100,
			},
		},
		{
			ID: "server-b",
			Capacity: models.ResourceCapacity{
				CPUCores:    10,
				MemoryBytes: 100,
			},
			PowerProfile: energy.PowerProfile{
				IdlePowerW:       10,
				CPUDynamicPowerW: 10,
			},
		},
		{
			ID: "server-c",
			Capacity: models.ResourceCapacity{
				CPUCores:    10,
				MemoryBytes: 100,
			},
			Allocated: models.ResourceCapacity{
				CPUCores: 8,
			},
			PowerProfile: energy.PowerProfile{
				IdlePowerW:       1,
				CPUDynamicPowerW: 1,
			},
		},
	}

	ranked, err := policy.Rank(workload, candidates)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) != 2 {
		t.Fatalf("expected 2 shortlisted nodes, got %d", len(ranked))
	}

	if ranked[0].ServerID != "server-b" {
		t.Errorf("expected server-b first after energy ranking, got %s", ranked[0].ServerID)
	}

	for _, result := range ranked {
		if result.ServerID == "server-c" {
			t.Error("server-c should not be included in the top-2 resource-aware shortlist")
		}

		if _, exists := result.Metrics["resource_aware_score"]; !exists {
			t.Errorf("resource-aware score missing for %s", result.ServerID)
		}
	}
}

func TestHybridAdaptiveDefaultsTopK(t *testing.T) {
	policy := &HybridAdaptivePolicy{}

	if policy.TopK != 0 {
		t.Fatal("expected zero-value TopK before default is applied")
	}

	workload := &models.Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 1,
		},
	}

	server := &models.Server{
		ID: "server-a",
		Capacity: models.ResourceCapacity{
			CPUCores:    10,
			MemoryBytes: 100,
		},
		PowerProfile: energy.PowerProfile{
			IdlePowerW: 10,
		},
	}

	ranked, err := policy.Rank(workload, []*models.Server{server})
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) != 1 {
		t.Fatalf("expected 1 result, got %d", len(ranked))
	}
}

func TestHybridAdaptiveRejectsNilWorkload(t *testing.T) {
	policy := &HybridAdaptivePolicy{TopK: 5}

	_, err := policy.Rank(nil, nil)

	if err == nil {
		t.Fatal("expected error for nil workload")
	}
}
