package main

import (
	"eawsk/simulator/energy"
	"eawsk/simulator/engine"
	"eawsk/simulator/models"
	"eawsk/simulator/policy"
	"eawsk/simulator/trace"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const (
	workloadFile = "data/processed/alibaba-gpu-v2026/pilot_5000.csv"
	serverFile   = "data/processed/alibaba-gpu-v2026/go_servers.csv"
	profileFile  = "configs/power_profiles_smoke_test.json"
	outputDir    = "results"
)

type Result struct {
	Policy         string
	Total          int
	Excluded       int
	Completed      int
	Unschedulable  int
	CompletionRate float64
	EnergyWh       float64
	MakespanSec    float64
}

func loadProfiles() (map[string]energy.PowerProfile, error) {
	data, err := os.ReadFile(profileFile)
	if err != nil {
		return nil, err
	}

	profiles := make(map[string]energy.PowerProfile)
	err = json.Unmarshal(data, &profiles)
	return profiles, err
}

func writeCSV(path string, header []string, rows [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return writer.Error()
}

func runPolicy(
	name string,
	schedulingPolicy policy.SchedulingPolicy,
	profiles map[string]energy.PowerProfile,
) (Result, error) {
	workloads, servers, err := trace.LoadCSV(workloadFile, serverFile, profiles)
	if err != nil {
		return Result{}, err
	}

	// Build the exact GPU models available in the server inventory.
	availableModels := make(map[string]bool)
	for _, server := range servers {
		availableModels[server.Hardware.GPUModel] = true
	}

	// Exclude workloads whose exact GPU model has no matching inventory.
	filtered := make([]*models.Workload, 0, len(workloads))
	excluded := 0

	for _, workload := range workloads {
		model := workload.Requirements.GPUModel
		if workload.Requirements.GPURequest > 0 && !availableModels[model] {
			excluded++
			continue
		}
		filtered = append(filtered, workload)
	}

	sim := engine.NewSimulation()

	if err := sim.SetPolicy(schedulingPolicy); err != nil {
		return Result{}, err
	}

	for _, server := range servers {
		if err := sim.AddServer(server); err != nil {
			return Result{}, err
		}
	}

	for _, workload := range filtered {
		if err := sim.AddWorkload(workload); err != nil {
			return Result{}, err
		}
	}

	// Process arrivals and completions until the event queue is empty.
	for {
		event, err := sim.ProcessNextEvent()
		if err != nil {
			return Result{}, err
		}
		if event == nil {
			break
		}
	}

	var totalEnergy float64
	for _, server := range sim.Servers {
		totalEnergy += server.TotalEnergyWh
	}

	completionRate := 0.0
	if len(filtered) > 0 {
		completionRate = float64(sim.CompletedWorkloads) / float64(len(filtered)) * 100
	}

	result := Result{
		Policy:         name,
		Total:          len(filtered),
		Excluded:       excluded,
		Completed:      sim.CompletedWorkloads,
		Unschedulable:  sim.UnschedulableWorkloads,
		CompletionRate: completionRate,
		EnergyWh:       totalEnergy,
		MakespanSec:    sim.CurrentTime.Seconds(),
	}

	// Export workload-level scheduling decisions.
	rows := make([][]string, 0, len(filtered))
	for _, workload := range filtered {
		rows = append(rows, []string{
			workload.ID,
			string(workload.AI.Type),
			workload.Requirements.GPUModel,
			string(workload.State),
			workload.AssignedServer,
			strconv.FormatFloat(workload.ArrivalTime.Seconds(), 'f', 3, 64),
			strconv.FormatFloat(workload.StartTime.Seconds(), 'f', 3, 64),
			strconv.FormatFloat(workload.CompletionTime.Seconds(), 'f', 3, 64),
		})
	}

	assignmentPath := filepath.Join(outputDir, name+"_assignments.csv")
	err = writeCSV(
		assignmentPath,
		[]string{"workload_id", "ai_type", "gpu_model", "state", "assigned_server",
			"arrival_sec", "start_sec", "completion_sec"},
		rows,
	)
	if err != nil {
		return Result{}, err
	}

	return result, nil
}

func main() {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		panic(err)
	}

	profiles, err := loadProfiles()
	if err != nil {
		panic(err)
	}

	policies := []struct {
		name   string
		policy policy.SchedulingPolicy
	}{
		{"first_fit", &policy.FirstFitPolicy{}},
		{"resource_aware", &policy.ResourceAwarePolicy{}},
		{"energy_aware", &policy.EnergyAwarePolicy{}},
		{"hybrid_adaptive", &policy.HybridAdaptivePolicy{TopK: 5}},
	}

	results := make([]Result, 0, len(policies))

	for _, item := range policies {
		fmt.Printf("\nRunning %s...\n", item.name)

		result, err := runPolicy(item.name, item.policy, profiles)
		if err != nil {
			panic(fmt.Errorf("%s: %w", item.name, err))
		}

		results = append(results, result)

		fmt.Printf(
			"%s: completed=%d, unschedulable=%d, completion=%.2f%%, energy=%.2f Wh\n",
			result.Policy,
			result.Completed,
			result.Unschedulable,
			result.CompletionRate,
			result.EnergyWh,
		)
	}

	rows := make([][]string, 0, len(results))
	for _, result := range results {
		rows = append(rows, []string{
			result.Policy,
			strconv.Itoa(result.Total),
			strconv.Itoa(result.Excluded),
			strconv.Itoa(result.Completed),
			strconv.Itoa(result.Unschedulable),
			strconv.FormatFloat(result.CompletionRate, 'f', 2, 64),
			strconv.FormatFloat(result.EnergyWh, 'f', 4, 64),
			strconv.FormatFloat(result.MakespanSec, 'f', 2, 64),
		})
	}

	err = writeCSV(
		filepath.Join(outputDir, "pilot_5000_policy_comparison.csv"),
		[]string{"policy", "included_workloads", "excluded_workloads",
			"completed", "unschedulable", "completion_rate_percent",
			"estimated_energy_wh", "makespan_seconds"},
		rows,
	)
	if err != nil {
		panic(err)
	}

	fmt.Println("\nResults saved in:", outputDir)
}
