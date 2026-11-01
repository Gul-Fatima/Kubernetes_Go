package models

import (
	"fmt"
	"time"
)

// AI workload categories derived from the trace.
type AIWorkloadType string

const (
	WorkloadTraining         AIWorkloadType = "training"
	WorkloadOnlineInference  AIWorkloadType = "online_inference"
	WorkloadOfflineInference AIWorkloadType = "offline_inference"
	WorkloadDevelopment      AIWorkloadType = "dev"
	WorkloadUnknown          AIWorkloadType = "unknown"
)

// WorkloadState represents the simulated workload lifecycle.
type WorkloadState string

const (
	StateQueued        WorkloadState = "queued"
	StateRunning       WorkloadState = "running"
	StateCompleted     WorkloadState = "completed"
	StateUnschedulable WorkloadState = "unschedulable"
)

// WorkloadRequirements describes the resources requested by a workload.
type WorkloadRequirements struct {
	CPURequestCores    float64 `json:"cpu_request_cores"`
	MemoryRequestBytes int64   `json:"memory_request_bytes"`
	GPURequest         float64     `json:"gpu_request"`
	GPUMemoryBytes     int64   `json:"gpu_memory_bytes"`

	// Optional hardware compatibility requirement.
	GPUModel string `json:"gpu_model,omitempty"`
}

// AIWorkloadInfo describes the workload's AI-specific characteristics.
type AIWorkloadInfo struct {
	Type          AIWorkloadType `json:"type"`
	ModelType     string         `json:"model_type,omitempty"`
	IsGenAI       bool           `json:"is_genai"`
	PriorityClass string         `json:"priority_class,omitempty"`
	WorkloadID    string         `json:"workload_id,omitempty"`
}

// Workload represents one unique workload submitted to the simulator.
type Workload struct {
	ID        string `json:"pod_id"`
	ClusterID string `json:"cluster_id"`

	Requirements WorkloadRequirements `json:"requirements"`
	AI           AIWorkloadInfo       `json:"ai"`

	ArrivalTime   time.Duration `json:"arrival_time"`
	ExecutionTime time.Duration `json:"execution_time"`

	State          WorkloadState `json:"state"`
	AssignedServer string        `json:"assigned_server,omitempty"`

	StartTime      time.Duration `json:"start_time,omitempty"`
	CompletionTime time.Duration `json:"completion_time,omitempty"`
}

// Validate checks whether the workload contains valid scheduling data.
func (w *Workload) Validate() error {
	if w.ID == "" {
		return fmt.Errorf("workload ID cannot be empty")
	}

	if w.ArrivalTime < 0 {
		return fmt.Errorf("workload %s has negative arrival time", w.ID)
	}

	if w.ExecutionTime <= 0 {
		return fmt.Errorf("workload %s must have positive execution time", w.ID)
	}

	r := w.Requirements

	if r.CPURequestCores < 0 ||
		r.MemoryRequestBytes < 0 ||
		r.GPURequest < 0 ||
		r.GPUMemoryBytes < 0 {
		return fmt.Errorf("workload %s has negative resource requirements", w.ID)
	}

	if w.AI.Type == "" {
		return fmt.Errorf("workload %s has no AI workload type", w.ID)
	}

	return nil
}
