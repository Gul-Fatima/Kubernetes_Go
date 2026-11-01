package trace

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"eawsk/simulator/energy"
	"eawsk/simulator/models"
)

// LoadCSV loads prepared Alibaba workloads and servers.
// Power profiles must be supplied separately, keyed by GPU model.
func LoadCSV(
	workloadPath string,
	serverPath string,
	powerProfiles map[string]energy.PowerProfile,
) ([]*models.Workload, []*models.Server, error) {
	workloads, err := loadWorkloads(workloadPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load workloads: %w", err)
	}

	servers, err := loadServers(serverPath, powerProfiles)
	if err != nil {
		return nil, nil, fmt.Errorf("load servers: %w", err)
	}

	return workloads, servers, nil
}

func openCSV(path string) (*csv.Reader, *os.File, map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		file.Close()
		return nil, nil, nil, err
	}

	columns := make(map[string]int, len(header))
	for i, name := range header {
		name = strings.TrimSpace(name)
		if _, exists := columns[name]; exists {
			file.Close()
			return nil, nil, nil, fmt.Errorf("duplicate CSV column %q", name)
		}
		columns[name] = i
	}

	return reader, file, columns, nil
}

func field(row []string, columns map[string]int, name string) (string, error) {
	index, exists := columns[name]
	if !exists {
		return "", fmt.Errorf("missing CSV column %q", name)
	}
	if index >= len(row) {
		return "", fmt.Errorf("missing value for column %q", name)
	}
	return strings.TrimSpace(row[index]), nil
}

func parseFloat(row []string, columns map[string]int, name string) (float64, error) {
	value, err := field(row, columns, name)
	if err != nil {
		return 0, err
	}
	result, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", name, err)
	}
	return result, nil
}

func parseInt64(row []string, columns map[string]int, name string) (int64, error) {
	value, err := field(row, columns, name)
	if err != nil {
		return 0, err
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("column %s: %w", name, err)
	}
	return result, nil
}

func secondsToDuration(seconds float64, column string) (time.Duration, error) {
	if seconds < 0 || seconds > float64((1<<63-1)/int64(time.Second)) {
		return 0, fmt.Errorf("invalid %s: %v seconds", column, seconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func workloadType(value string) models.AIWorkloadType {
	switch strings.ToLower(value) {
	case "training":
		return models.WorkloadTraining
	case "online_inference":
		return models.WorkloadOnlineInference
	case "offline_inference":
		return models.WorkloadOfflineInference
	case "dev":
		return models.WorkloadDevelopment
	default:
		return models.WorkloadUnknown
	}
}

func loadWorkloads(path string) ([]*models.Workload, error) {
	reader, file, columns, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	workloads := make([]*models.Workload, 0)
	seen := make(map[string]struct{})

	for rowNumber := 2; ; rowNumber++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		id, err := field(row, columns, "id")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("row %d: duplicate workload ID %q", rowNumber, id)
		}
		seen[id] = struct{}{}

		jobType, err := field(row, columns, "job_type")
		if err != nil {
			return nil, err
		}
		modelType, err := field(row, columns, "model_type")
		if err != nil {
			return nil, err
		}
		gpuModel, err := field(row, columns, "gpu_model")
		if err != nil {
			return nil, err
		}

		cpu, err := parseFloat(row, columns, "cpu_request_cores")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		memory, err := parseInt64(row, columns, "memory_request_bytes")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		gpu, err := parseFloat(row, columns, "gpu_request")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		gpuMemory, err := parseInt64(row, columns, "gpu_memory_request_bytes")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		arrivalSeconds, err := parseFloat(row, columns, "arrival_time_seconds")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		executionSeconds, err := parseFloat(row, columns, "execution_time_seconds")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		arrival, err := secondsToDuration(arrivalSeconds, "arrival_time_seconds")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		execution, err := secondsToDuration(executionSeconds, "execution_time_seconds")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		requirements := models.WorkloadRequirements{
			CPURequestCores:    cpu,
			MemoryRequestBytes: memory,
			GPURequest:         gpu,
			GPUMemoryBytes:     gpuMemory,
		}

		// GPU model compatibility only applies to GPU workloads.
		if gpu > 0 {
			requirements.GPUModel = gpuModel
		}

		workload := &models.Workload{
			ID:        id,
			ClusterID: "default",
			Requirements: requirements,
			AI: models.AIWorkloadInfo{
				Type:      workloadType(jobType),
				ModelType: modelType,
				IsGenAI:   strings.EqualFold(modelType, "genai"),
			},
			ArrivalTime:   arrival,
			ExecutionTime: execution,
			State:         models.StateQueued,
		}

		if err := workload.Validate(); err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		workloads = append(workloads, workload)
	}

	return workloads, nil
}

func loadServers(
	path string,
	powerProfiles map[string]energy.PowerProfile,
) ([]*models.Server, error) {
	reader, file, columns, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	servers := make([]*models.Server, 0)
	seen := make(map[string]struct{})

	for rowNumber := 2; ; rowNumber++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		id, err := field(row, columns, "id")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("row %d: duplicate server ID %q", rowNumber, id)
		}
		seen[id] = struct{}{}

		clusterID, err := field(row, columns, "cluster_id")
		if err != nil {
			return nil, err
		}
		gpuModel, err := field(row, columns, "gpu_model")
		if err != nil {
			return nil, err
		}

		cpuCores, err := parseFloat(row, columns, "cpu_cores")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		memoryBytes, err := parseInt64(row, columns, "memory_bytes")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		gpuCount, err := parseInt64(row, columns, "gpu_count")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}
		gpuMemoryBytes, err := parseInt64(row, columns, "gpu_memory_bytes")
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		if gpuCount < 0 || gpuCount > int64(int(^uint(0)>>1)) {
			return nil, fmt.Errorf("row %d: invalid GPU count", rowNumber)
		}

		profile, exists := powerProfiles[gpuModel]
		if !exists {
			return nil, fmt.Errorf(
				"row %d: no power profile supplied for GPU model %q",
				rowNumber,
				gpuModel,
			)
		}

		server := &models.Server{
			ID:        id,
			ClusterID: clusterID,
			Hardware: models.HardwareSpec{
				GPUModel: gpuModel,
				GPUCount: int(gpuCount),
				// CPU model, physical/logical core split, GPU
				// architecture and per-device GPU memory are unavailable.
			},
			PowerProfile: profile,
			Capacity: models.ResourceCapacity{
				CPUCores:       cpuCores,
				MemoryBytes:    memoryBytes,
				GPUCount:       float64(gpuCount),
				GPUMemoryBytes: gpuMemoryBytes,
			},
		}

		if err := server.Validate(); err != nil {
			return nil, fmt.Errorf("row %d: %w", rowNumber, err)
		}

		servers = append(servers, server)
	}

	return servers, nil
}
