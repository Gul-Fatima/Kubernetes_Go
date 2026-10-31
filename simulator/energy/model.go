package energy

import (
	"fmt"
	"time"
)

// PowerProfile contains hardware-specific power parameters.
// Dynamic power values represent additional power above idle.
type PowerProfile struct {
	IdlePowerW             float64 `json:"idle_power_w"`
	CPUDynamicPowerW       float64 `json:"cpu_dynamic_power_w"`
	GPUDynamicPowerWPerGPU float64 `json:"gpu_dynamic_power_w_per_gpu"`
	MemoryDynamicPowerW    float64 `json:"memory_dynamic_power_w"`

	Source          string `json:"source"`
	CalibrationInfo string `json:"calibration_info,omitempty"`
}

// Utilization contains normalized resource utilization values (0–1).
type Utilization struct {
	CPU         float64 `json:"cpu"`
	GPU         float64 `json:"gpu"`
	Memory      float64 `json:"memory"`
	MemoryKnown bool    `json:"memory_known"`
}

// EnergyEstimate records the result and its provenance.
type EnergyEstimate struct {
	PowerW   float64       `json:"power_w"`
	EnergyWh float64       `json:"energy_wh"`
	Duration time.Duration `json:"duration"`
	Method   string        `json:"method"`
	Source   string        `json:"source"`
}

// EstimatePower calculates server power using a component-based
// linear utilization model.
func EstimatePower(
	profile PowerProfile,
	util Utilization,
	gpuCount int,
) (float64, error) {
	if profile.IdlePowerW < 0 ||
		profile.CPUDynamicPowerW < 0 ||
		profile.GPUDynamicPowerWPerGPU < 0 ||
		profile.MemoryDynamicPowerW < 0 {
		return 0, fmt.Errorf("power parameters cannot be negative")
	}

	if gpuCount < 0 {
		return 0, fmt.Errorf("GPU count cannot be negative")
	}

	values := []float64{util.CPU, util.GPU, util.Memory}
	for _, value := range values {
		if value < 0 || value > 1 {
			return 0, fmt.Errorf("utilization must be between 0 and 1")
		}
	}

	power := profile.IdlePowerW +
		profile.CPUDynamicPowerW*util.CPU +
		profile.GPUDynamicPowerWPerGPU*float64(gpuCount)*util.GPU

	if util.MemoryKnown {
		power += profile.MemoryDynamicPowerW * util.Memory
	}

	return power, nil
}

// EstimateEnergy calculates energy consumption over a duration.
// Energy is returned in watt-hours (Wh).
func EstimateEnergy(
	profile PowerProfile,
	util Utilization,
	gpuCount int,
	duration time.Duration,
) (EnergyEstimate, error) {
	if duration <= 0 {
		return EnergyEstimate{}, fmt.Errorf("duration must be positive")
	}

	power, err := EstimatePower(profile, util, gpuCount)
	if err != nil {
		return EnergyEstimate{}, err
	}

	method := "linear_component_utilization"
	if !util.MemoryKnown {
		method = "linear_cpu_gpu_utilization_memory_unknown"
	}

	hours := duration.Hours()

	return EnergyEstimate{
		PowerW:   power,
		EnergyWh: power * hours,
		Duration: duration,
		Method:   method,
		Source:   profile.Source,
	}, nil
}
