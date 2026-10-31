package policy

import (
	"testing"
	"time"

	"eawsk/simulator/energy"
	"eawsk/simulator/models"
)

func TestEnergyAwarePrefersLowerIncrementalEnergy(t *testing.T) {
	policy := &EnergyAwarePolicy{}

	workload := &models.Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 2,
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
				CPUDynamicPowerW: 200,
			},
		},
		{
			ID: "server-b",
			Capacity: models.ResourceCapacity{
				CPUCores:    10,
				MemoryBytes: 100,
			},
			PowerProfile: energy.PowerProfile{
				IdlePowerW:       50,
				CPUDynamicPowerW: 100,
			},
		},
	}

	ranked, err := policy.Rank(workload, candidates)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(ranked))
	}

	if ranked[0].ServerID != "server-b" {
		t.Errorf("expected server-b first, got %s", ranked[0].ServerID)
	}
}

func TestEnergyAwareRejectsNilWorkload(t *testing.T) {
	policy := &EnergyAwarePolicy{}

	_, err := policy.Rank(nil, []*models.Server{})

	if err == nil {
		t.Fatal("expected error for nil workload")
	}
}
