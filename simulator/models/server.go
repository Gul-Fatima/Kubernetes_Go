package models

import (
	"fmt"
	"time"

	"eawsk/simulator/energy"
)

// ResourceCapacity represents CPU, memory and GPU resources.
// Memory values are stored in bytes.
type ResourceCapacity struct {
	CPUCores       float64 `json:"cpu_cores"`
	MemoryBytes    int64   `json:"memory_bytes"`
	GPUCount       float64 `json:"gpu_count"`
	GPUMemoryBytes int64   `json:"gpu_memory_bytes"`
}

// HardwareSpec describes the physical hardware of a server.
type HardwareSpec struct {
	CPUModel            string `json:"cpu_model"`
	CPUPhysicalCores    int    `json:"cpu_physical_cores"`
	CPULogicalCores     int    `json:"cpu_logical_cores"`
	MemoryCapacityBytes int64  `json:"memory_capacity_bytes"`

	GPUModel                string `json:"gpu_model"`
	GPUArchitecture         string `json:"gpu_architecture"`
	GPUCount                int    `json:"gpu_count"`
	GPUMemoryPerDeviceBytes int64  `json:"gpu_memory_per_device_bytes"`
}

// Server represents a virtual or physical compute server.
type Server struct {
	ID        string `json:"server_id"`
	ClusterID string `json:"cluster_id"`

	Hardware         HardwareSpec        `json:"hardware"`
	PowerProfile     energy.PowerProfile `json:"power_profile"`
	CurrentPowerW    float64             `json:"current_power_w"`
	TotalEnergyWh    float64             `json:"total_energy_wh"`
	LastEnergyUpdate time.Duration       `json:"last_energy_update"`

	Capacity  ResourceCapacity `json:"capacity"`
	Allocated ResourceCapacity `json:"allocated"`

	RunningWorkloads []string `json:"running_workloads"`
}

// AvailableResources returns the resources currently available.
func (s *Server) AvailableResources() ResourceCapacity {
	return ResourceCapacity{
		CPUCores:       s.Capacity.CPUCores - s.Allocated.CPUCores,
		MemoryBytes:    s.Capacity.MemoryBytes - s.Allocated.MemoryBytes,
		GPUCount:       s.Capacity.GPUCount - s.Allocated.GPUCount,
		GPUMemoryBytes: s.Capacity.GPUMemoryBytes - s.Allocated.GPUMemoryBytes,
	}
}

// Validate checks whether the server's hardware and capacity are valid.
func (s *Server) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("server ID cannot be empty")
	}

	if s.Capacity.CPUCores <= 0 {
		return fmt.Errorf("server %s must have positive CPU capacity", s.ID)
	}

	// Zero means host memory capacity is unknown.
	if s.Capacity.MemoryBytes < 0 {
		return fmt.Errorf("server memory capacity cannot be negative")
	}

	if s.Capacity.GPUCount < 0 || s.Capacity.GPUMemoryBytes < 0 {
		return fmt.Errorf("server %s has invalid GPU capacity", s.ID)
	}

	if s.Allocated.CPUCores < 0 ||
		s.Allocated.MemoryBytes < 0 ||
		s.Allocated.GPUCount < 0 ||
		s.Allocated.GPUMemoryBytes < 0 {
		return fmt.Errorf("server %s has negative allocated resources", s.ID)
	}

	available := s.AvailableResources()

	if available.CPUCores < 0 ||
		available.MemoryBytes < 0 ||
		available.GPUCount < 0 ||
		available.GPUMemoryBytes < 0 {
		return fmt.Errorf("server %s is over-allocated", s.ID)
	}

	return nil
}

// UpdateEnergy estimates server energy over the elapsed simulation interval.
// It assumes the supplied utilization was constant since the previous update.
func (s *Server) UpdateEnergy(util energy.Utilization, now time.Duration) error {
	if now < s.LastEnergyUpdate {
		return fmt.Errorf(
			"server %s: simulation time cannot move backwards",
			s.ID,
		)
	}

	power, err := energy.EstimatePower(
		s.PowerProfile,
		util,
		s.Hardware.GPUCount,
	)
	if err != nil {
		return fmt.Errorf("server %s: %w", s.ID, err)
	}

	elapsed := now - s.LastEnergyUpdate

	if elapsed == 0 {
		s.CurrentPowerW = power
		return nil
	}

	estimate, err := energy.EstimateEnergy(
		s.PowerProfile,
		util,
		s.Hardware.GPUCount,
		elapsed,
	)
	if err != nil {
		return fmt.Errorf("server %s: %w", s.ID, err)
	}

	s.CurrentPowerW = estimate.PowerW
	s.TotalEnergyWh += estimate.EnergyWh
	s.LastEnergyUpdate = now

	return nil
}

// AllocateWorkload reserves resources for a workload on this server.
func (s *Server) AllocateWorkload(workload *Workload) error {
	if workload == nil {
		return fmt.Errorf("workload cannot be nil")
	}

	if err := s.Validate(); err != nil {
		return err
	}

	if err := workload.Validate(); err != nil {
		return err
	}

	// Prevent duplicate allocation.
	for _, id := range s.RunningWorkloads {
		if id == workload.ID {
			return fmt.Errorf(
				"workload %s is already allocated on server %s",
				workload.ID,
				s.ID,
			)
		}
	}

	// Check GPU model compatibility.
	if workload.Requirements.GPURequest > 0 &&
		workload.Requirements.GPUModel != "" &&
		workload.Requirements.GPUModel != s.Hardware.GPUModel {
		return fmt.Errorf(
			"server %s does not have the required GPU model",
			s.ID,
		)
	}

	available := s.AvailableResources()
	req := workload.Requirements

	if req.MemoryRequestBytes > 0 && s.Capacity.MemoryBytes <= 0 {
		return fmt.Errorf("server %s has unknown memory capacity", s.ID)
	}

	if req.CPURequestCores > available.CPUCores ||
		req.MemoryRequestBytes > available.MemoryBytes ||
		req.GPURequest > available.GPUCount ||
		req.GPUMemoryBytes > available.GPUMemoryBytes {
		return fmt.Errorf(
			"insufficient resources on server %s for workload %s",
			s.ID,
			workload.ID,
		)
	}

	// Reserve resources.
	s.Allocated.CPUCores += req.CPURequestCores
	s.Allocated.MemoryBytes += req.MemoryRequestBytes
	s.Allocated.GPUCount += req.GPURequest
	s.Allocated.GPUMemoryBytes += req.GPUMemoryBytes

	s.RunningWorkloads = append(s.RunningWorkloads, workload.ID)

	return nil
}

// ReleaseWorkload releases resources previously allocated to a workload.
func (s *Server) ReleaseWorkload(workload *Workload) error {
	if workload == nil {
		return fmt.Errorf("workload cannot be nil")
	}

	if err := workload.Validate(); err != nil {
		return err
	}

	// Find the workload in the running workload list.
	index := -1
	for i, id := range s.RunningWorkloads {
		if id == workload.ID {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf(
			"workload %s is not running on server %s",
			workload.ID,
			s.ID,
		)
	}

	req := workload.Requirements

	// Ensure resources can be safely released.
	const epsilon = 1e-9
	if s.Allocated.CPUCores+epsilon < req.CPURequestCores ||
		s.Allocated.MemoryBytes < req.MemoryRequestBytes ||
		s.Allocated.GPUCount+epsilon < req.GPURequest ||
		s.Allocated.GPUMemoryBytes < req.GPUMemoryBytes {
		return fmt.Errorf(
			"allocated resources are insufficient to release workload %s",
			workload.ID,
		)
	}

	// Release resources.
	s.Allocated.CPUCores -= req.CPURequestCores
	s.Allocated.MemoryBytes -= req.MemoryRequestBytes
	s.Allocated.GPUCount -= req.GPURequest
	s.Allocated.GPUMemoryBytes -= req.GPUMemoryBytes

	// Remove workload from running list.
	s.RunningWorkloads = append(
		s.RunningWorkloads[:index],
		s.RunningWorkloads[index+1:]...,
	)

	return nil
}
