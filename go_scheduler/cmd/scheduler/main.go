package main

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"

	configv1 "k8s.io/kube-scheduler/config/v1"

	"k8s.io/kubernetes/pkg/scheduler"
	schedulerconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/apis/config/scheme"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	"k8s.io/kubernetes/pkg/scheduler/profile"

	"eawsk/go_scheduler/plugins/energyscore"
)

func makeNode(
	name, cpu, memory, gpu string,
	idlePower, cpuPower, gpuPower float64,
) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				energyscore.IdlePowerAnnotation:   fmt.Sprint(idlePower),
				energyscore.CPUPowerAnnotation:    fmt.Sprint(cpuPower),
				energyscore.GPUPowerAnnotation:    fmt.Sprint(gpuPower),
				energyscore.MemoryPowerAnnotation: "30",
				energyscore.PowerSourceAnnotation: "illustrative-test-profile",
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:      resource.MustParse(cpu),
				corev1.ResourceMemory:   resource.MustParse(memory),
				corev1.ResourcePods:     resource.MustParse("110"),
				energyscore.GPUResource: resource.MustParse(gpu),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:      resource.MustParse(cpu),
				corev1.ResourceMemory:   resource.MustParse(memory),
				corev1.ResourcePods:     resource.MustParse("110"),
				energyscore.GPUResource: resource.MustParse(gpu),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

func makePod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-test-pod",
			Namespace: "default",
			UID:       types.UID("gpu-test-pod-uid"),
		},
		Spec: corev1.PodSpec{
			SchedulerName: "default-scheduler",
			Containers: []corev1.Container{
				{
					Name:    "gpu-container",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:      resource.MustParse("2"),
							corev1.ResourceMemory:   resource.MustParse("4Gi"),
							energyscore.GPUResource: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}
}

func defaultProfile(enableEnergyScore bool) ([]schedulerconfig.KubeSchedulerProfile, error) {
	var versioned configv1.KubeSchedulerConfiguration

	// Generate the actual Kubernetes v1.37.0 default scheduler configuration.
	scheme.Scheme.Default(&versioned)

	internal := schedulerconfig.KubeSchedulerConfiguration{}
	if err := scheme.Scheme.Convert(&versioned, &internal, nil); err != nil {
		return nil, fmt.Errorf("converting default scheduler configuration: %w", err)
	}

	if len(internal.Profiles) == 0 {
		return nil, fmt.Errorf("Kubernetes generated no scheduler profiles")
	}

	defaultProfile := &internal.Profiles[0]

	if defaultProfile.Plugins == nil {
		return nil, fmt.Errorf("default scheduler profile has no plugin configuration")
	}

	if enableEnergyScore {
		defaultProfile.Plugins.Score.Enabled = append(
			defaultProfile.Plugins.Score.Enabled,
			schedulerconfig.Plugin{
				Name:   energyscore.Name,
				Weight: 1,
			},
		)
	}

	return internal.Profiles, nil
}

func main() {
	mode := "energy"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode != "baseline" && mode != "energy" {
		panic("usage: scheduler-test [baseline|energy]")
	}
	enableEnergyScore := mode == "energy"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := []*corev1.Node{
		makeNode("node-a", "8", "32Gi", "1", 100, 150, 250),
		makeNode("node-b", "16", "64Gi", "4", 120, 200, 300),
	}

	objects := make([]runtime.Object, 0, len(nodes))
	for _, node := range nodes {
		objects = append(objects, node)
	}

	client := fake.NewSimpleClientset(objects...)

	informerFactory := informers.NewSharedInformerFactory(client, 0)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(
		dynamicClient, 0,
	)

	broadcaster := events.NewBroadcaster(
		&events.EventSinkImpl{Interface: client.EventsV1()},
	)
	broadcaster.StartRecordingToSink(ctx.Done())
	defer broadcaster.Shutdown()

	recorderFactory := profile.NewRecorderFactory(broadcaster)

	fmt.Printf("[1] Building scheduler profile (mode=%s)...\n", mode)
	profiles, err := defaultProfile(enableEnergyScore)
	if err != nil {
		panic(err)
	}

	registry := frameworkruntime.Registry{
		energyscore.Name: energyscore.New,
	}

	fmt.Println("[2] Creating Kubernetes scheduler...")
	sched, err := scheduler.New(
		ctx,
		client,
		informerFactory,
		dynamicInformerFactory,
		recorderFactory,
		scheduler.WithProfiles(profiles[0]),
		scheduler.WithFrameworkOutOfTreeRegistry(registry),
	)
	if err != nil {
		panic(fmt.Errorf("creating scheduler: %w", err))
	}
	fmt.Println("[2] Scheduler created successfully.")

	fmt.Println("[3] Starting informers...")
	informerFactory.Start(ctx.Done())
	dynamicInformerFactory.Start(ctx.Done())

	fmt.Println("[4] Waiting for informer synchronization...")
	if err := sched.WaitForHandlersSync(ctx); err != nil {
		panic(fmt.Errorf("waiting for informer handlers: %w", err))
	}
	fmt.Println("[4] Informers synchronized successfully.")

	fmt.Println("[5] Starting scheduler loop...")
	go sched.Run(ctx)

	fmt.Println("[6] Creating test Pod...")
	pod := makePod()
	_, err = client.CoreV1().Pods(pod.Namespace).Create(
		ctx, pod, metav1.CreateOptions{},
	)
	if err != nil {
		panic(fmt.Errorf("creating Pod: %w", err))
	}

	fmt.Printf("Submitted Pod: %s/%s\n", pod.Namespace, pod.Name)
	fmt.Println("Waiting for Kubernetes scheduling decision...")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(15 * time.Second)

	for {
		select {
		case <-timeout:
			fmt.Println("Timed out waiting for a scheduling decision.")
			for _, action := range client.Actions() {
				fmt.Printf("%s %s subresource=%s\n",
					action.GetVerb(),
					action.GetResource(),
					action.GetSubresource(),
				)
			}
			return

		case <-ticker.C:
			for _, action := range client.Actions() {
				if action.GetVerb() != "create" ||
					action.GetSubresource() != "binding" {
					continue
				}

				createAction, ok := action.(ktesting.CreateAction)
				if !ok {
					continue
				}

				binding, ok := createAction.GetObject().(*corev1.Binding)
				if !ok {
					continue
				}

				fmt.Println("\nScheduling result")
				fmt.Println("-----------------")
				fmt.Println("Pod:", binding.Namespace+"/"+binding.Name)
				fmt.Println("Node:", binding.Target.Name)
				fmt.Println("Scheduler: default-scheduler")
				return
			}
		}
	}
}
