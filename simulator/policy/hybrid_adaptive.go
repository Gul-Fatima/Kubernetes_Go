package policy

import (
	"fmt"

	"eawsk/simulator/models"
)

const DefaultHybridTopK = 5

type HybridAdaptivePolicy struct {
	TopK int
}

func (p *HybridAdaptivePolicy) Name() string {
	return "hybrid_adaptive"
}

func (p *HybridAdaptivePolicy) Rank(
	workload *models.Workload,
	candidates []*models.Server,
) ([]RankedServer, error) {
	if workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	if len(candidates) == 0 {
		return []RankedServer{}, nil
	}

	topK := p.TopK
	if topK <= 0 {
		topK = DefaultHybridTopK
	}

	// Stage 1: Rank all feasible nodes by resource efficiency.
	resourcePolicy := &ResourceAwarePolicy{}

	resourceRanked, err := resourcePolicy.Rank(workload, candidates)
	if err != nil {
		return nil, fmt.Errorf("resource-aware ranking failed: %w", err)
	}

	if len(resourceRanked) == 0 {
		return []RankedServer{}, nil
	}

	if topK > len(resourceRanked) {
		topK = len(resourceRanked)
	}

	// Build a lookup of candidate servers.
	serverMap := make(map[string]*models.Server, len(candidates))
	for _, server := range candidates {
		if server != nil {
			serverMap[server.ID] = server
		}
	}

	// Stage 2: Shortlist the top K resource-efficient nodes.
	shortlisted := make([]*models.Server, 0, topK)
	resourceScores := make(map[string]float64, topK)

	for _, ranked := range resourceRanked[:topK] {
		server, exists := serverMap[ranked.ServerID]
		if !exists {
			continue
		}

		shortlisted = append(shortlisted, server)
		resourceScores[ranked.ServerID] = ranked.Score
	}

	// Stage 3: Rank shortlisted nodes by estimated incremental energy.
	energyPolicy := &EnergyAwarePolicy{}

	energyRanked, err := energyPolicy.Rank(workload, shortlisted)
	if err != nil {
		return nil, fmt.Errorf("energy-aware ranking failed: %w", err)
	}

	// Preserve the resource-aware score for analysis.
	for i := range energyRanked {
		if energyRanked[i].Metrics == nil {
			energyRanked[i].Metrics = make(map[string]float64)
		}

		energyRanked[i].Metrics["resource_aware_score"] =
			resourceScores[energyRanked[i].ServerID]

		energyRanked[i].Reason =
			"energy ranking among top resource-efficient candidates"
	}

	return energyRanked, nil
}
