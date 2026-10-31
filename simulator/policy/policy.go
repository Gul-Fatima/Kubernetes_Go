package policy

import (
	"eawsk/simulator/models"
)

type RankedServer struct {
	ServerID string
	Score    float64
	Reason   string
	Metrics  map[string]float64
}

type SchedulingPolicy interface {
	Name() string

	Rank(
		workload *models.Workload,
		candidates []*models.Server,
	) ([]RankedServer, error)
}
