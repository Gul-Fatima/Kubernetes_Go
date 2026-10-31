package policy

import (
	"fmt"

	"eawsk/simulator/models"
)

type FirstFitPolicy struct{}

func (p *FirstFitPolicy) Name() string {
	return "first_fit"
}

func (p *FirstFitPolicy) Rank(
	workload *models.Workload,
	candidates []*models.Server,
) ([]RankedServer, error) {

	if workload == nil {
		return nil, fmt.Errorf("workload cannot be nil")
	}

	if len(candidates) == 0 {
		return []RankedServer{}, nil
	}

	ranked := make([]RankedServer, 0, len(candidates))

	for i, server := range candidates {
		if server == nil {
			continue
		}

		// First candidate receives the highest priority.
		ranked = append(ranked, RankedServer{
			ServerID: server.ID,
			Score:    float64(len(candidates) - i),
			Reason:   "first-fit candidate order",
		})
	}

	return ranked, nil
}
