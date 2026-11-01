package engine

import (
	"fmt"
	"sort"
	"time"

	"eawsk/simulator/energy"
	"eawsk/simulator/models"
	"eawsk/simulator/policy"
)

// Simulation holds the complete state of one simulation run.
type Simulation struct {
	CurrentTime time.Duration

	Servers           map[string]*models.Server
	ServersByGPUModel map[string][]*models.Server
	Workloads         map[string]*models.Workload

	Events *EventScheduler

	Policy policy.SchedulingPolicy

	CompletedWorkloads     int
	UnschedulableWorkloads int
}

// NewSimulation creates an initialized simulation instance.
func NewSimulation() *Simulation {
	return &Simulation{
		CurrentTime:       0,
		Servers:           make(map[string]*models.Server),
		ServersByGPUModel: make(map[string][]*models.Server),
		Workloads:         make(map[string]*models.Workload),
		Events:            NewEventScheduler(),
	}
}

// SetPolicy configures the scheduling policy for this simulation.
func (s *Simulation) SetPolicy(p policy.SchedulingPolicy) error {
	if p == nil {
		return fmt.Errorf("scheduling policy cannot be nil")
	}

	s.Policy = p
	return nil
}

// AddServer registers a server in the simulation.
func (s *Simulation) AddServer(server *models.Server) error {
	if server == nil {
		return fmt.Errorf("server cannot be nil")
	}

	if err := server.Validate(); err != nil {
		return err
	}

	if _, exists := s.Servers[server.ID]; exists {
		return fmt.Errorf("server %s already exists", server.ID)
	}

	s.Servers[server.ID] = server
	if server.Hardware.GPUModel != "" {
		s.ServersByGPUModel[server.Hardware.GPUModel] = append(
			s.ServersByGPUModel[server.Hardware.GPUModel], server,
		)
	}
	return nil
}

// AddWorkload registers a workload and schedules its arrival.
func (s *Simulation) AddWorkload(workload *models.Workload) error {
	if workload == nil {
		return fmt.Errorf("workload cannot be nil")
	}

	if err := workload.Validate(); err != nil {
		return err
	}

	if _, exists := s.Workloads[workload.ID]; exists {
		return fmt.Errorf("workload %s already exists", workload.ID)
	}

	if workload.ArrivalTime < s.CurrentTime {
		return fmt.Errorf(
			"workload %s arrival time is before current simulation time",
			workload.ID,
		)
	}

	if workload.State != "" && workload.State != models.StateQueued {
		return fmt.Errorf(
			"workload %s must be queued before registration",
			workload.ID,
		)
	}

	workload.State = models.StateQueued
	s.Workloads[workload.ID] = workload

	err := s.Events.Schedule(Event{
		Time:       workload.ArrivalTime,
		Type:       EventWorkloadArrival,
		WorkloadID: workload.ID,
	})
	if err != nil {
		delete(s.Workloads, workload.ID)
		return err
	}

	return nil
}

// FeasibleServers returns servers that can satisfy the workload requirements.
func (s *Simulation) FeasibleServers(
	workload *models.Workload,
) ([]*models.Server, error) {
	if workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	if err := workload.Validate(); err != nil {
		return nil, err
	}

	candidates := make([]*models.Server, 0)

	// GPU workloads inspect only servers with the exact required GPU model.
	serverPool := make([]*models.Server, 0, len(s.Servers))
	if workload.Requirements.GPURequest > 0 &&
		workload.Requirements.GPUModel != "" {
		serverPool = s.ServersByGPUModel[workload.Requirements.GPUModel]
	} else {
		for _, server := range s.Servers {
			serverPool = append(serverPool, server)
		}
	}

	for _, server := range serverPool {
		if server == nil {
			continue
		}

		available := server.AvailableResources()
		requirements := workload.Requirements

		if requirements.CPURequestCores > available.CPUCores {
			continue
		}

		// Unknown host memory cannot establish feasibility for
		// workloads that request memory.
		if requirements.MemoryRequestBytes > 0 &&
			server.Capacity.MemoryBytes <= 0 {
			continue
		}

		if requirements.MemoryRequestBytes > available.MemoryBytes {
			continue
		}

		if requirements.GPURequest > available.GPUCount {
			continue
		}

		if requirements.GPUMemoryBytes > available.GPUMemoryBytes {
			continue
		}

		if requirements.GPURequest > 0 &&
			requirements.GPUModel != "" &&
			requirements.GPUModel != server.Hardware.GPUModel {
			continue
		}

		candidates = append(candidates, server)
	}

	// Ensure deterministic candidate order for policies such as First Fit.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ID < candidates[j].ID
	})

	return candidates, nil
}

// RankWorkload filters feasible servers and ranks them using the configured policy.
func (s *Simulation) RankWorkload(
	workload *models.Workload,
) ([]policy.RankedServer, error) {
	if s.Policy == nil {
		return nil, fmt.Errorf("no scheduling policy configured")
	}

	candidates, err := s.FeasibleServers(workload)
	if err != nil {
		return nil, err
	}

	return s.Policy.Rank(workload, candidates)
}

// Advance processes the next event and updates simulation time.
func (s *Simulation) Advance() (*Event, error) {
	event := s.Events.Next()
	if event == nil {
		return nil, nil
	}

	if event.Time < s.CurrentTime {
		return nil, fmt.Errorf(
			"event time is before current simulation time",
		)
	}

	s.CurrentTime = event.Time

	return event, nil
}

// serverUtilization estimates utilization from allocated resource requests.
// These are simulation estimates, not measured hardware utilization.
func serverUtilization(server *models.Server) energy.Utilization {
	const epsilon = 1e-9

	normalize := func(allocated, capacity float64) float64 {
		if capacity <= 0 {
			return 0
		}

		util := allocated / capacity

		// Correct negligible floating-point deviations only.
		if util < 0 && util >= -epsilon {
			return 0
		}
		if util > 1 && util <= 1+epsilon {
			return 1
		}
		return util
	}

	util := energy.Utilization{
		CPU: normalize(server.Allocated.CPUCores, server.Capacity.CPUCores),
	}

	if server.Capacity.MemoryBytes > 0 {
		util.Memory = normalize(
			float64(server.Allocated.MemoryBytes),
			float64(server.Capacity.MemoryBytes),
		)
		util.MemoryKnown = true
	}

	if server.Capacity.GPUCount > 0 {
		util.GPU = normalize(
			server.Allocated.GPUCount,
			server.Capacity.GPUCount,
		)
	}

	return util
}

// ProcessEvent handles a simulation event.
func (s *Simulation) ProcessEvent(event *Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	switch event.Type {
	case EventWorkloadArrival:
		return s.handleWorkloadArrival(event)

	case EventWorkloadCompletion:
		return s.handleWorkloadCompletion(event)

	case EventSimulationEnd:
		return nil

	default:
		return fmt.Errorf("unsupported event type: %s", event.Type)
	}
}

// handleWorkloadArrival schedules an arriving workload.
func (s *Simulation) handleWorkloadArrival(event *Event) error {
	workload, exists := s.Workloads[event.WorkloadID]
	if !exists {
		return fmt.Errorf("workload %s not found", event.WorkloadID)
	}

	if workload.State != models.StateQueued {
		return fmt.Errorf("workload %s is not queued", workload.ID)
	}

	ranked, err := s.RankWorkload(workload)
	if err != nil {
		return err
	}

	if len(ranked) == 0 {
		workload.State = models.StateUnschedulable
		s.UnschedulableWorkloads++
		return nil
	}

	// Try candidates in policy ranking order.
	for _, candidate := range ranked {
		server, exists := s.Servers[candidate.ServerID]
		if !exists {
			continue
		}

		// Account for energy using utilization before allocation.
		if err := server.UpdateEnergy(
			serverUtilization(server),
			s.CurrentTime,
		); err != nil {
			return err
		}

		if err := server.AllocateWorkload(workload); err != nil {
			continue
		}

		// Update current power after allocation.
		if err := server.UpdateEnergy(
			serverUtilization(server),
			s.CurrentTime,
		); err != nil {
			_ = server.ReleaseWorkload(workload)
			return err
		}

		completionTime := s.CurrentTime + workload.ExecutionTime

		// Schedule completion.
		err := s.Events.Schedule(Event{
			Time:       completionTime,
			Type:       EventWorkloadCompletion,
			WorkloadID: workload.ID,
			ServerID:   server.ID,
		})
		if err != nil {
			_ = server.ReleaseWorkload(workload)
			return err
		}

		workload.State = models.StateRunning
		workload.AssignedServer = server.ID
		workload.StartTime = s.CurrentTime
		workload.CompletionTime = completionTime

		return nil
	}

	// No candidate could accept the workload.
	workload.State = models.StateUnschedulable
	s.UnschedulableWorkloads++

	return nil
}

// handleWorkloadCompletion completes a running workload and releases resources.
func (s *Simulation) handleWorkloadCompletion(event *Event) error {
	workload, exists := s.Workloads[event.WorkloadID]
	if !exists {
		return fmt.Errorf("workload %s not found", event.WorkloadID)
	}

	if workload.State != models.StateRunning {
		return fmt.Errorf("workload %s is not running", workload.ID)
	}

	if workload.AssignedServer != event.ServerID {
		return fmt.Errorf(
			"workload %s is not assigned to server %s",
			workload.ID,
			event.ServerID,
		)
	}

	server, exists := s.Servers[event.ServerID]
	if !exists {
		return fmt.Errorf("server %s not found", event.ServerID)
	}

	// Account for energy before releasing resources.
	if err := server.UpdateEnergy(
		serverUtilization(server),
		s.CurrentTime,
	); err != nil {
		return err
	}

	if err := server.ReleaseWorkload(workload); err != nil {
		return err
	}

	// Update current power after resource release.
	if err := server.UpdateEnergy(
		serverUtilization(server),
		s.CurrentTime,
	); err != nil {
		return err
	}

	workload.State = models.StateCompleted
	s.CompletedWorkloads++

	return nil
}

// ProcessNextEvent advances simulation time and processes the next event.
func (s *Simulation) ProcessNextEvent() (*Event, error) {
	event, err := s.Advance()
	if err != nil || event == nil {
		return event, err
	}

	if err := s.ProcessEvent(event); err != nil {
		return event, err
	}

	return event, nil
}
