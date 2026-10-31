package energy

import (
	"math"
	"testing"
	"time"
)

func TestEstimatePower(t *testing.T) {
	profile := PowerProfile{
		IdlePowerW:             100,
		CPUDynamicPowerW:       200,
		GPUDynamicPowerWPerGPU: 300,
		MemoryDynamicPowerW:    50,
	}

	util := Utilization{
		CPU:         0.5,
		GPU:         0.8,
		Memory:      0.4,
		MemoryKnown: true,
	}

	// 100 + (200*0.5) + (300*2*0.8) + (50*0.4) = 700 W
	got, err := EstimatePower(profile, util, 2)
	if err != nil {
		t.Fatal(err)
	}

	if math.Abs(got-700) > 0.001 {
		t.Fatalf("expected 700 W, got %.2f W", got)
	}
}

func TestEstimatePowerWithUnknownMemory(t *testing.T) {
	profile := PowerProfile{
		IdlePowerW:          100,
		CPUDynamicPowerW:    200,
		MemoryDynamicPowerW: 50,
	}

	util := Utilization{
		CPU:    0.5,
		Memory: 0.4,
	}

	got, err := EstimatePower(profile, util, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Memory contribution is excluded because memory capacity is unknown.
	expected := 100 + (200 * 0.5)

	if math.Abs(got-expected) > 0.001 {
		t.Fatalf("expected %.2f W, got %.2f W", expected, got)
	}
}

func TestEstimateEnergy(t *testing.T) {
	profile := PowerProfile{
		IdlePowerW: 100,
		Source:     "unit-test",
	}

	result, err := EstimateEnergy(
		profile,
		Utilization{},
		0,
		2*time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}

	if result.EnergyWh != 200 {
		t.Fatalf("expected 200 Wh, got %.2f Wh", result.EnergyWh)
	}

	if result.Method != "linear_cpu_gpu_utilization_memory_unknown" {
		t.Fatalf("unexpected method: %s", result.Method)
	}
}

func TestInvalidUtilization(t *testing.T) {
	_, err := EstimatePower(
		PowerProfile{},
		Utilization{CPU: 1.5},
		0,
	)

	if err == nil {
		t.Fatal("expected error for utilization above 1")
	}
}
