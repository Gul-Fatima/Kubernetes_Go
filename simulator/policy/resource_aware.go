package policy

import (
	"fmt"
	"sort"

	"eawsk/simulator/models"
)

type ResourceAwarePolicy struct{}

func (p *ResourceAwarePolicy) Name() string {
	return "resource_aware"
}

func (p *ResourceAwarePolicy) Rank(
	workload *models.Workload,
	candidates []*models.Server,
) ([]RankedServer, error) {
	if workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	ranked := make([]RankedServer, 0, len(candidates))

	for _, server := range candidates {
		if server == nil {
			continue
		}

		if server.Capacity.CPUCores <= 0 {
			return nil, fmt.Errorf(
				"server %s has invalid CPU capacity",
				server.ID,
			)
		}

		req := workload.Requirements

		if req.MemoryRequestBytes > 0 && server.Capacity.MemoryBytes <= 0 {
			return nil, fmt.Errorf(
				"server %s has unknown memory capacity for a memory-requiring workload",
				server.ID,
			)
		}

		cpuUtil := (server.Allocated.CPUCores + req.CPURequestCores) /
			server.Capacity.CPUCores

		metrics := map[string]float64{
			"projected_cpu_utilization": cpuUtil,
		}

		totalUtil := cpuUtil
		dimensions := 1

		// Include host memory only when its capacity is known.
		if server.Capacity.MemoryBytes > 0 {
			memoryUtil := float64(
				server.Allocated.MemoryBytes+req.MemoryRequestBytes,
			) / float64(server.Capacity.MemoryBytes)

			metrics["projected_memory_utilization"] = memoryUtil
			totalUtil += memoryUtil
			dimensions++
		}

		if server.Capacity.GPUCount > 0 {
			gpuUtil := float64(
				server.Allocated.GPUCount+req.GPURequest,
			) / float64(server.Capacity.GPUCount)

			metrics["projected_gpu_utilization"] = gpuUtil
			totalUtil += gpuUtil
			dimensions++

			if server.Capacity.GPUMemoryBytes > 0 {
				gpuMemoryUtil := float64(
					server.Allocated.GPUMemoryBytes+req.GPUMemoryBytes,
				) / float64(server.Capacity.GPUMemoryBytes)

				metrics["projected_gpu_memory_utilization"] = gpuMemoryUtil
				totalUtil += gpuMemoryUtil
				dimensions++
			}
		}

		averageUtil := totalUtil / float64(dimensions)
		metrics["projected_average_utilization"] = averageUtil

		ranked = append(ranked, RankedServer{
			ServerID: server.ID,
			Score:    -averageUtil,
			Reason:   "lowest projected average resource utilization",
			Metrics:  metrics,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked, nil
}
