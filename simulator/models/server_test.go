package models

import (
	"testing"
	"time"

	"eawsk/simulator/energy"
)

func TestUpdateEnergy(t *testing.T) {
	server := Server{
		ID: "server-1",
		Capacity: ResourceCapacity{
			CPUCores:    8,
			MemoryBytes: 16 * 1024 * 1024 * 1024,
			GPUCount:    2,
		},
		PowerProfile: energy.PowerProfile{
			IdlePowerW:             100,
			CPUDynamicPowerW:       200,
			GPUDynamicPowerWPerGPU: 300,
			MemoryDynamicPowerW:    50,
			Source:                 "test",
		},
	}

	util := energy.Utilization{
		CPU:         0.5,
		GPU:         0.8,
		Memory:      0.4,
		MemoryKnown: true,
	}

	err := server.UpdateEnergy(util, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// Expected power: 700 W.
	// Expected energy: 350 Wh over 30 minutes.
	if server.CurrentPowerW != 700 {
		t.Errorf("expected 700 W, got %.2f W", server.CurrentPowerW)
	}

	if server.TotalEnergyWh != 350 {
		t.Errorf("expected 350 Wh, got %.2f Wh", server.TotalEnergyWh)
	}
}

func TestAllocateWorkload(t *testing.T) {
	server := &Server{
		ID: "server-1",
		Capacity: ResourceCapacity{
			CPUCores:       16,
			MemoryBytes:    32 * 1024 * 1024 * 1024,
			GPUCount:       2,
			GPUMemoryBytes: 48 * 1024 * 1024 * 1024,
		},
	}

	workload := &Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: WorkloadRequirements{
			CPURequestCores:    4,
			MemoryRequestBytes: 8 * 1024 * 1024 * 1024,
			GPURequest:         1,
			GPUMemoryBytes:     16 * 1024 * 1024 * 1024,
		},
		AI: AIWorkloadInfo{Type: WorkloadTraining},
	}

	if err := server.AllocateWorkload(workload); err != nil {
		t.Fatal(err)
	}

	if server.Allocated.CPUCores != 4 ||
		server.Allocated.MemoryBytes != 8*1024*1024*1024 ||
		server.Allocated.GPUCount != 1 ||
		server.Allocated.GPUMemoryBytes != 16*1024*1024*1024 {
		t.Fatal("allocated resources do not match workload requirements")
	}

	if len(server.RunningWorkloads) != 1 ||
		server.RunningWorkloads[0] != workload.ID {
		t.Fatal("workload was not registered as running")
	}
}

func TestReleaseWorkload(t *testing.T) {
	server := &Server{
		ID: "server-1",
		Capacity: ResourceCapacity{
			CPUCores:       16,
			MemoryBytes:    32 * 1024 * 1024 * 1024,
			GPUCount:       2,
			GPUMemoryBytes: 48 * 1024 * 1024 * 1024,
		},
	}

	workload := &Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: WorkloadRequirements{
			CPURequestCores:    4,
			MemoryRequestBytes: 8 * 1024 * 1024 * 1024,
			GPURequest:         1,
			GPUMemoryBytes:     16 * 1024 * 1024 * 1024,
		},
		AI: AIWorkloadInfo{Type: WorkloadTraining},
	}

	if err := server.AllocateWorkload(workload); err != nil {
		t.Fatal(err)
	}

	if err := server.ReleaseWorkload(workload); err != nil {
		t.Fatal(err)
	}

	if server.Allocated != (ResourceCapacity{}) {
		t.Fatalf("expected all resources released, got %+v", server.Allocated)
	}

	if len(server.RunningWorkloads) != 0 {
		t.Fatal("running workload list should be empty")
	}
}

func TestAllocateWorkloadInsufficientResources(t *testing.T) {
	server := &Server{
		ID: "server-1",
		Capacity: ResourceCapacity{
			CPUCores:    4,
			MemoryBytes: 8 * 1024 * 1024 * 1024,
		},
	}

	workload := &Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: WorkloadRequirements{
			CPURequestCores: 8,
		},
		AI: AIWorkloadInfo{Type: WorkloadTraining},
	}

	if err := server.AllocateWorkload(workload); err == nil {
		t.Fatal("expected insufficient resources error")
	}

	if server.Allocated != (ResourceCapacity{}) {
		t.Fatal("resources should remain unchanged after failed allocation")
	}
}

func TestDuplicateWorkloadAllocation(t *testing.T) {
	server := &Server{
		ID: "server-1",
		Capacity: ResourceCapacity{
			CPUCores:    16,
			MemoryBytes: 32 * 1024 * 1024 * 1024,
		},
	}

	workload := &Workload{
		ID:            "workload-1",
		ExecutionTime: time.Hour,
		Requirements: WorkloadRequirements{
			CPURequestCores: 2,
		},
		AI: AIWorkloadInfo{Type: WorkloadTraining},
	}

	if err := server.AllocateWorkload(workload); err != nil {
		t.Fatal(err)
	}

	if err := server.AllocateWorkload(workload); err == nil {
		t.Fatal("expected duplicate allocation error")
	}
}
