package policy

import (
	"testing"

	"eawsk/simulator/models"
)

func TestResourceAwarePrefersLowerUtilization(t *testing.T) {
	policy := &ResourceAwarePolicy{}

	workload := &models.Workload{
		ID: "workload-1",
		Requirements: models.WorkloadRequirements{
			CPURequestCores:    2,
			MemoryRequestBytes: 2,
		},
	}

	candidates := []*models.Server{
		{
			ID: "server-a",
			Capacity: models.ResourceCapacity{
				CPUCores:    10,
				MemoryBytes: 10,
			},
			Allocated: models.ResourceCapacity{
				CPUCores:    6,
				MemoryBytes: 6,
			},
		},
		{
			ID: "server-b",
			Capacity: models.ResourceCapacity{
				CPUCores:    10,
				MemoryBytes: 10,
			},
			Allocated: models.ResourceCapacity{
				CPUCores:    1,
				MemoryBytes: 1,
			},
		},
	}

	ranked, err := policy.Rank(workload, candidates)
	if err != nil {
		t.Fatal(err)
	}

	if ranked[0].ServerID != "server-b" {
		t.Errorf("expected server-b first, got %s", ranked[0].ServerID)
	}
}

func TestResourceAwareIncludesGPUMemory(t *testing.T) {
	policy := &ResourceAwarePolicy{}

	workload := &models.Workload{
		ID: "gpu-workload",
		Requirements: models.WorkloadRequirements{
			GPURequest:     1,
			GPUMemoryBytes: 4,
		},
	}

	server := &models.Server{
		ID: "gpu-server",
		Capacity: models.ResourceCapacity{
			CPUCores:       10,
			MemoryBytes:    100,
			GPUCount:       2,
			GPUMemoryBytes: 16,
		},
	}

	ranked, err := policy.Rank(workload, []*models.Server{server})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := ranked[0].Metrics["projected_gpu_memory_utilization"]; !ok {
		t.Fatal("GPU memory utilization metric is missing")
	}

	if ranked[0].Metrics["projected_gpu_memory_utilization"] != 0.25 {
		t.Errorf("expected GPU memory utilization 0.25, got %f",
			ranked[0].Metrics["projected_gpu_memory_utilization"])
	}
}
