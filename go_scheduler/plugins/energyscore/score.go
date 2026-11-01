package energyscore

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	fwk "k8s.io/kube-scheduler/framework"

	"eawsk/simulator/energy"
)

const (
	AnnotationPrefix = "eawsk.io/"

	IdlePowerAnnotation   = AnnotationPrefix + "idle-power-w"
	CPUPowerAnnotation    = AnnotationPrefix + "cpu-dynamic-power-w"
	GPUPowerAnnotation    = AnnotationPrefix + "gpu-dynamic-power-w-per-gpu"
	MemoryPowerAnnotation = AnnotationPrefix + "memory-dynamic-power-w"
	PowerSourceAnnotation = AnnotationPrefix + "power-source"

	GPUResource corev1.ResourceName = "nvidia.com/gpu"

	defaultReferencePowerW = 1000.0
)

func annotationFloat(
	node *corev1.Node,
	key string,
	required bool,
) (float64, error) {
	value, exists := node.Annotations[key]

	if !exists {
		if required {
			return 0, fmt.Errorf("node %s is missing annotation %s", node.Name, key)
		}
		return 0, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"node %s has invalid annotation %s: %w",
			node.Name, key, err,
		)
	}

	return parsed, nil
}

func podRequests(pod *corev1.Pod) (cpuMilli, memoryBytes, gpu int64) {
	for _, container := range pod.Spec.Containers {
		requests := container.Resources.Requests

		cpuMilli += requests.Cpu().MilliValue()
		memoryBytes += requests.Memory().Value()
		gpu += requests.Name(GPUResource, resource.DecimalSI).Value()
	}

	return
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func scoreNode(
	pod *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) (int64, *fwk.Status) {
	node := nodeInfo.Node()
	if node == nil {
		return 0, fwk.NewStatus(fwk.Error, "node information is unavailable")
	}

	idlePower, err := annotationFloat(node, IdlePowerAnnotation, true)
	if err != nil {
		return 0, fwk.NewStatus(fwk.Error, err.Error())
	}

	cpuPower, err := annotationFloat(node, CPUPowerAnnotation, true)
	if err != nil {
		return 0, fwk.NewStatus(fwk.Error, err.Error())
	}

	gpuPower, err := annotationFloat(node, GPUPowerAnnotation, true)
	if err != nil {
		return 0, fwk.NewStatus(fwk.Error, err.Error())
	}

	memoryPower, err := annotationFloat(node, MemoryPowerAnnotation, false)
	if err != nil {
		return 0, fwk.NewStatus(fwk.Error, err.Error())
	}

	cpuRequest, memoryRequest, gpuRequest := podRequests(pod)

	allocatable := nodeInfo.GetAllocatable()
	requested := nodeInfo.GetRequested()

	if allocatable == nil || requested == nil {
		return 0, fwk.NewStatus(fwk.Error, "node resource information is unavailable")
	}

	cpuCapacity := allocatable.GetMilliCPU()
	memoryCapacity := allocatable.GetMemory()
	gpuCapacity := allocatable.GetScalarResources()[GPUResource]

	if cpuCapacity <= 0 {
		return 0, fwk.NewStatus(fwk.Error, "node has no allocatable CPU")
	}

	projectedCPU := requested.GetMilliCPU() + cpuRequest
	projectedMemory := requested.GetMemory() + memoryRequest
	projectedGPU := requested.GetScalarResources()[GPUResource] + gpuRequest

	cpuUtil := clamp(float64(projectedCPU) / float64(cpuCapacity))

	gpuUtil := 0.0
	gpuCount := int(gpuCapacity)
	if gpuCapacity > 0 {
		gpuUtil = clamp(float64(projectedGPU) / float64(gpuCapacity))
	}

	memoryUtil := 0.0
	memoryKnown := memoryCapacity > 0
	if memoryKnown {
		memoryUtil = clamp(float64(projectedMemory) / float64(memoryCapacity))
	}

	profile := energy.PowerProfile{
		IdlePowerW:             idlePower,
		CPUDynamicPowerW:       cpuPower,
		GPUDynamicPowerWPerGPU: gpuPower,
		MemoryDynamicPowerW:    memoryPower,
		Source:                 node.Annotations[PowerSourceAnnotation],
	}

	if profile.Source == "" {
		profile.Source = "node-annotations"
	}

	utilization := energy.Utilization{
		CPU:         cpuUtil,
		GPU:         gpuUtil,
		Memory:      memoryUtil,
		MemoryKnown: memoryKnown,
	}

	powerW, err := energy.EstimatePower(profile, utilization, gpuCount)
	if err != nil {
		return 0, fwk.NewStatus(fwk.Error, err.Error())
	}

	// Monotonic score: lower estimated power receives a higher score.
	score := 100.0 * defaultReferencePowerW /
		(defaultReferencePowerW + powerW)

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return int64(score + 0.5), nil
}
