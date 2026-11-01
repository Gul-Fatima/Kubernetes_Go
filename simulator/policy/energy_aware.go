package policy

import (
	"fmt"
	"sort"

	"eawsk/simulator/energy"
	"eawsk/simulator/models"
)

type EnergyAwarePolicy struct{}

func (p *EnergyAwarePolicy) Name() string {
	return "energy_aware"
}

func (p *EnergyAwarePolicy) Rank(
	workload *models.Workload,
	candidates []*models.Server,
) ([]RankedServer, error) {
	if workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	if workload.ExecutionTime <= 0 {
		return nil, fmt.Errorf("workload execution time must be positive")
	}

	ranked := make([]RankedServer, 0, len(candidates))

	for _, server := range candidates {
		if server == nil {
			continue
		}

		if err := server.Validate(); err != nil {
			return nil, err
		}

		capacity := server.Capacity
		allocated := server.Allocated
		requirements := workload.Requirements

		if requirements.MemoryRequestBytes > 0 && capacity.MemoryBytes <= 0 {
			return nil, fmt.Errorf(
				"server %s has unknown memory capacity for a memory-requiring workload",
				server.ID,
			)
		}

		currentUtil := energy.Utilization{
			CPU: allocated.CPUCores / capacity.CPUCores,
		}

		projectedUtil := energy.Utilization{
			CPU: (allocated.CPUCores + requirements.CPURequestCores) /
				capacity.CPUCores,
		}

		// Include memory utilization only when host RAM capacity is known.
		if capacity.MemoryBytes > 0 {
			currentUtil.Memory = float64(allocated.MemoryBytes) /
				float64(capacity.MemoryBytes)
			currentUtil.MemoryKnown = true

			projectedUtil.Memory = float64(
				allocated.MemoryBytes+requirements.MemoryRequestBytes,
			) / float64(capacity.MemoryBytes)
			projectedUtil.MemoryKnown = true
		}

		if capacity.GPUCount > 0 {
			currentUtil.GPU = float64(allocated.GPUCount) /
				float64(capacity.GPUCount)

			projectedUtil.GPU = float64(
				allocated.GPUCount+requirements.GPURequest,
			) / float64(capacity.GPUCount)
		}

		currentPower, err := energy.EstimatePower(
			server.PowerProfile,
			currentUtil,
			server.Hardware.GPUCount,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"server %s current power: %w",
				server.ID,
				err,
			)
		}

		projectedEstimate, err := energy.EstimateEnergy(
			server.PowerProfile,
			projectedUtil,
			server.Hardware.GPUCount,
			workload.ExecutionTime,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"server %s projected energy: %w",
				server.ID,
				err,
			)
		}

		currentEnergy := currentPower * workload.ExecutionTime.Hours()

		incrementalEnergy := projectedEstimate.EnergyWh - currentEnergy
		incrementalPower := projectedEstimate.PowerW - currentPower

		ranked = append(ranked, RankedServer{
			ServerID: server.ID,
			Score:    -incrementalEnergy,
			Reason:   "lowest estimated incremental energy consumption",
			Metrics: map[string]float64{
				"current_power_w":       currentPower,
				"projected_power_w":     projectedEstimate.PowerW,
				"incremental_power_w":   incrementalPower,
				"incremental_energy_wh": incrementalEnergy,
				"projected_energy_wh":   projectedEstimate.EnergyWh,
			},
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked, nil
}
