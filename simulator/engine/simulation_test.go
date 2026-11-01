package engine

import (
	"eawsk/simulator/energy"
	"eawsk/simulator/models"
	"eawsk/simulator/policy"
	"testing"
	"time"
)

func TestAddServer(t *testing.T) {
	sim := NewSimulation()

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    16,
			MemoryBytes: 32 * 1024 * 1024 * 1024,
			GPUCount:    2,
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	if sim.Servers["server-1"] != server {
		t.Fatal("server was not registered")
	}

	if err := sim.AddServer(server); err == nil {
		t.Fatal("expected duplicate server error")
	}
}

func TestAddWorkload(t *testing.T) {
	sim := NewSimulation()

	workload := &models.Workload{
		ID:            "pod-1",
		ArrivalTime:   10 * time.Minute,
		ExecutionTime: 30 * time.Minute,
		Requirements: models.WorkloadRequirements{
			CPURequestCores:    2,
			MemoryRequestBytes: 4 * 1024 * 1024 * 1024,
			GPURequest:         1,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	if err := sim.AddWorkload(workload); err != nil {
		t.Fatal(err)
	}

	if workload.State != models.StateQueued {
		t.Fatal("workload should be queued")
	}

	if sim.Events.Len() != 1 {
		t.Fatal("workload arrival event was not scheduled")
	}

	if err := sim.AddWorkload(workload); err == nil {
		t.Fatal("expected duplicate workload error")
	}
}

func TestAdvanceSimulationClock(t *testing.T) {
	sim := NewSimulation()

	err := sim.Events.Schedule(Event{
		Time: 15 * time.Minute,
		Type: EventWorkloadArrival,
	})

	if err != nil {
		t.Fatal(err)
	}

	event, err := sim.Advance()
	if err != nil {
		t.Fatal(err)
	}

	if event == nil {
		t.Fatal("expected an event")
	}

	if sim.CurrentTime != 15*time.Minute {
		t.Fatalf("expected 15 minutes, got %v", sim.CurrentTime)
	}
}
func TestFeasibleServers(t *testing.T) {
	sim := NewSimulation()

	servers := []*models.Server{
		{
			ID: "server-1",
			Capacity: models.ResourceCapacity{
				CPUCores:    16,
				MemoryBytes: 32 * 1024 * 1024 * 1024,
				GPUCount:    2,
			},
		},
		{
			ID: "server-2",
			Capacity: models.ResourceCapacity{
				CPUCores:    2,
				MemoryBytes: 4 * 1024 * 1024 * 1024,
				GPUCount:    0,
			},
		},
	}

	for _, server := range servers {
		if err := sim.AddServer(server); err != nil {
			t.Fatal(err)
		}
	}

	workload := &models.Workload{
		ID:            "pod-1",
		ExecutionTime: 30 * time.Minute,
		Requirements: models.WorkloadRequirements{
			CPURequestCores:    4,
			MemoryRequestBytes: 8 * 1024 * 1024 * 1024,
			GPURequest:         1,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	feasible, err := sim.FeasibleServers(workload)
	if err != nil {
		t.Fatal(err)
	}

	if len(feasible) != 1 {
		t.Fatalf("expected 1 feasible server, got %d", len(feasible))
	}

	if feasible[0].ID != "server-1" {
		t.Errorf("expected server-1, got %s", feasible[0].ID)
	}
}

func TestRankWorkloadWithoutPolicy(t *testing.T) {
	sim := NewSimulation()

	workload := &models.Workload{
		ID:            "pod-1",
		ExecutionTime: time.Hour,
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	_, err := sim.RankWorkload(workload)

	if err == nil {
		t.Fatal("expected error when no policy is configured")
	}
}

func TestRankWorkloadWithPolicy(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    16,
			MemoryBytes: 32 * 1024 * 1024 * 1024,
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	workload := &models.Workload{
		ID:            "pod-1",
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 2,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	ranked, err := sim.RankWorkload(workload)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) != 1 {
		t.Fatalf("expected 1 ranked server, got %d", len(ranked))
	}

	if ranked[0].ServerID != "server-1" {
		t.Errorf("expected server-1, got %s", ranked[0].ServerID)
	}
}
func TestWorkloadArrivalAndCompletion(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    16,
			MemoryBytes: 32 * 1024 * 1024 * 1024,
		},
		PowerProfile: energy.PowerProfile{
			IdlePowerW:          50,
			CPUDynamicPowerW:    100,
			MemoryDynamicPowerW: 50,
			Source:              "test",
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	workload := &models.Workload{
		ID:            "pod-1",
		ArrivalTime:   time.Minute,
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores:    8,
			MemoryRequestBytes: 16 * 1024 * 1024 * 1024,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	if err := sim.AddWorkload(workload); err != nil {
		t.Fatal(err)
	}

	// Process arrival.
	event, err := sim.ProcessNextEvent()
	if err != nil {
		t.Fatal(err)
	}

	if event.Type != EventWorkloadArrival {
		t.Fatalf("expected arrival event, got %s", event.Type)
	}

	if workload.State != models.StateRunning {
		t.Fatalf("expected running state, got %s", workload.State)
	}

	if workload.AssignedServer != server.ID {
		t.Fatalf("expected server-1, got %s", workload.AssignedServer)
	}

	if server.Allocated.CPUCores != 8 {
		t.Fatalf("expected 8 allocated CPU cores, got %v", server.Allocated.CPUCores)
	}

	if len(server.RunningWorkloads) != 1 {
		t.Fatalf("expected 1 running workload, got %d", len(server.RunningWorkloads))
	}

	// Process completion.
	event, err = sim.ProcessNextEvent()
	if err != nil {
		t.Fatal(err)
	}

	if event.Type != EventWorkloadCompletion {
		t.Fatalf("expected completion event, got %s", event.Type)
	}

	if workload.State != models.StateCompleted {
		t.Fatalf("expected completed state, got %s", workload.State)
	}

	if server.Allocated.CPUCores != 0 {
		t.Fatalf("expected CPU allocation to return to 0, got %v", server.Allocated.CPUCores)
	}

	if len(server.RunningWorkloads) != 0 {
		t.Fatalf("expected no running workloads, got %d", len(server.RunningWorkloads))
	}
	if server.CurrentPowerW != 50 {
		t.Fatalf("expected idle power of 50W after completion, got %.2fW",
			server.CurrentPowerW)
	}
	if sim.CompletedWorkloads != 1 {
		t.Fatalf("expected 1 completed workload, got %d", sim.CompletedWorkloads)
	}

	// At 50% CPU and 50% memory utilization:
	// Power = 50 + (100 * 0.5) + (50 * 0.5) = 125 W.
	// Energy = 125 W * 1 hour = 125 Wh.
	//expectedEnergy := 125.0
	expectedEnergy := 125.0 + (50.0 / 60.0)

	//if server.TotalEnergyWh != expectedEnergy {
	//	t.Fatalf("expected %.2f Wh, got %.2f Wh", expectedEnergy, server.TotalEnergyWh)
	//}
	if diff := server.TotalEnergyWh - expectedEnergy; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("expected %.4f Wh, got %.4f Wh", expectedEnergy, server.TotalEnergyWh)
	}
}

func TestUnschedulableWorkloadArrival(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    2,
			MemoryBytes: 4 * 1024 * 1024 * 1024,
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	workload := &models.Workload{
		ID:            "pod-1",
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 8,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	if err := sim.AddWorkload(workload); err != nil {
		t.Fatal(err)
	}

	_, err := sim.ProcessNextEvent()
	if err != nil {
		t.Fatal(err)
	}

	if workload.State != models.StateUnschedulable {
		t.Fatalf("expected unschedulable state, got %s", workload.State)
	}

	if sim.UnschedulableWorkloads != 1 {
		t.Fatalf("expected 1 unschedulable workload, got %d", sim.UnschedulableWorkloads)
	}

	if len(server.RunningWorkloads) != 0 {
		t.Fatal("unschedulable workload should not be allocated")
	}

	if sim.Events.Len() != 0 {
		t.Fatal("unschedulable workload should not have a completion event")
	}
}
func TestMultipleWorkloadsShareServer(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    8,
			MemoryBytes: 8 * 1024 * 1024 * 1024,
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"pod-1", "pod-2"} {
		workload := &models.Workload{
			ID:            id,
			ArrivalTime:   0,
			ExecutionTime: time.Hour,
			Requirements: models.WorkloadRequirements{
				CPURequestCores:    4,
				MemoryRequestBytes: 2 * 1024 * 1024 * 1024,
			},
			AI: models.AIWorkloadInfo{
				Type: models.WorkloadTraining,
			},
		}

		if err := sim.AddWorkload(workload); err != nil {
			t.Fatal(err)
		}
	}

	// Process both arrivals.
	for i := 0; i < 2; i++ {
		if _, err := sim.ProcessNextEvent(); err != nil {
			t.Fatal(err)
		}
	}

	if server.Allocated.CPUCores != 8 {
		t.Fatalf("expected 8 allocated CPU cores, got %v", server.Allocated.CPUCores)
	}

	if len(server.RunningWorkloads) != 2 {
		t.Fatalf("expected 2 running workloads, got %d", len(server.RunningWorkloads))
	}

	// Process both completions.
	for i := 0; i < 2; i++ {
		if _, err := sim.ProcessNextEvent(); err != nil {
			t.Fatal(err)
		}
	}

	if server.Allocated.CPUCores != 0 {
		t.Fatalf("expected 0 allocated CPU cores, got %v", server.Allocated.CPUCores)
	}

	if sim.CompletedWorkloads != 2 {
		t.Fatalf("expected 2 completed workloads, got %d", sim.CompletedWorkloads)
	}
}
func TestCompletionBeforeArrivalAtSameTime(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    4,
			MemoryBytes: 8 * 1024 * 1024 * 1024,
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	first := &models.Workload{
		ID:            "pod-1",
		ArrivalTime:   0,
		ExecutionTime: 10 * time.Minute,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 4,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	second := &models.Workload{
		ID:            "pod-2",
		ArrivalTime:   10 * time.Minute,
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 4,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	if err := sim.AddWorkload(first); err != nil {
		t.Fatal(err)
	}

	if err := sim.AddWorkload(second); err != nil {
		t.Fatal(err)
	}

	// First workload arrives and occupies the server.
	if _, err := sim.ProcessNextEvent(); err != nil {
		t.Fatal(err)
	}

	if first.State != models.StateRunning {
		t.Fatalf("expected first workload to be running, got %s", first.State)
	}

	// At 10 minutes, first workload completes before second arrives.
	event, err := sim.ProcessNextEvent()
	if err != nil {
		t.Fatal(err)
	}

	if event.Type != EventWorkloadCompletion {
		t.Fatalf("expected completion event first, got %s", event.Type)
	}

	if first.State != models.StateCompleted {
		t.Fatalf("expected first workload completed, got %s", first.State)
	}

	// Second workload should now be able to use the released resources.
	event, err = sim.ProcessNextEvent()
	if err != nil {
		t.Fatal(err)
	}

	if event.Type != EventWorkloadArrival {
		t.Fatalf("expected arrival event second, got %s", event.Type)
	}

	if second.State != models.StateRunning {
		t.Fatalf("expected second workload running, got %s", second.State)
	}

	if sim.UnschedulableWorkloads != 0 {
		t.Fatalf("expected 0 unschedulable workloads, got %d", sim.UnschedulableWorkloads)
	}
}
func TestEnergyAccountingWithOverlappingWorkloads(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "server-1",
		Capacity: models.ResourceCapacity{
			CPUCores:    8,
			MemoryBytes: 8 * 1024 * 1024 * 1024,
		},
		PowerProfile: energy.PowerProfile{
			IdlePowerW:          50,
			CPUDynamicPowerW:    100,
			MemoryDynamicPowerW: 50,
			Source:              "test",
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	workloads := []*models.Workload{
		{
			ID:            "pod-1",
			ArrivalTime:   0,
			ExecutionTime: time.Hour,
			Requirements: models.WorkloadRequirements{
				CPURequestCores:    4,
				MemoryRequestBytes: 2 * 1024 * 1024 * 1024,
			},
			AI: models.AIWorkloadInfo{Type: models.WorkloadTraining},
		},
		{
			ID:            "pod-2",
			ArrivalTime:   30 * time.Minute,
			ExecutionTime: time.Hour,
			Requirements: models.WorkloadRequirements{
				CPURequestCores:    4,
				MemoryRequestBytes: 2 * 1024 * 1024 * 1024,
			},
			AI: models.AIWorkloadInfo{Type: models.WorkloadTraining},
		},
	}

	for _, workload := range workloads {
		if err := sim.AddWorkload(workload); err != nil {
			t.Fatal(err)
		}
	}

	// Process both arrivals and both completions.
	for i := 0; i < 4; i++ {
		if _, err := sim.ProcessNextEvent(); err != nil {
			t.Fatal(err)
		}
	}

	expectedEnergy := 200.0

	if diff := server.TotalEnergyWh - expectedEnergy; diff < -0.0001 || diff > 0.0001 {
		t.Fatalf("expected %.4f Wh, got %.4f Wh",
			expectedEnergy, server.TotalEnergyWh)
	}

	if sim.CompletedWorkloads != 2 {
		t.Fatalf("expected 2 completed workloads, got %d",
			sim.CompletedWorkloads)
	}

	if server.Allocated.CPUCores != 0 {
		t.Fatalf("expected 0 allocated CPU cores, got %v",
			server.Allocated.CPUCores)
	}
}
func TestFractionalGPUAllocationAndRelease(t *testing.T) {
	sim := NewSimulation()

	if err := sim.SetPolicy(&policy.FirstFitPolicy{}); err != nil {
		t.Fatal(err)
	}

	server := &models.Server{
		ID: "gpu-server",
		Capacity: models.ResourceCapacity{
			CPUCores:    16,
			MemoryBytes: 32 * 1024 * 1024 * 1024,
			GPUCount:    2,
		},
		Hardware: models.HardwareSpec{
			GPUCount: 2,
		},
	}

	if err := sim.AddServer(server); err != nil {
		t.Fatal(err)
	}

	workload := &models.Workload{
		ID:            "fractional-gpu-pod",
		ArrivalTime:   0,
		ExecutionTime: time.Hour,
		Requirements: models.WorkloadRequirements{
			CPURequestCores: 2,
			GPURequest:      0.5,
		},
		AI: models.AIWorkloadInfo{
			Type: models.WorkloadTraining,
		},
	}

	if err := sim.AddWorkload(workload); err != nil {
		t.Fatal(err)
	}

	// Process arrival.
	if _, err := sim.ProcessNextEvent(); err != nil {
		t.Fatal(err)
	}

	if workload.State != models.StateRunning {
		t.Fatalf("expected running state, got %s", workload.State)
	}

	if server.Allocated.GPUCount != 0.5 {
		t.Fatalf("expected 0.5 GPU allocated, got %v", server.Allocated.GPUCount)
	}

	if server.AvailableResources().GPUCount != 1.5 {
		t.Fatalf("expected 1.5 GPUs available, got %v",
			server.AvailableResources().GPUCount)
	}

	// Process completion.
	if _, err := sim.ProcessNextEvent(); err != nil {
		t.Fatal(err)
	}

	if workload.State != models.StateCompleted {
		t.Fatalf("expected completed state, got %s", workload.State)
	}

	if server.Allocated.GPUCount != 0 {
		t.Fatalf("expected GPU allocation to return to 0, got %v",
			server.Allocated.GPUCount)
	}

	if server.AvailableResources().GPUCount != 2 {
		t.Fatalf("expected 2 GPUs available after completion, got %v",
			server.AvailableResources().GPUCount)
	}
}
