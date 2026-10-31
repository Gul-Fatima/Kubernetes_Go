package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/events"

	"k8s.io/kubernetes/pkg/scheduler"
	"k8s.io/kubernetes/pkg/scheduler/profile"
)

func makeNode(name string, cpu, memory, gpu string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse(cpu),
				corev1.ResourceMemory:           resource.MustParse(memory),
				corev1.ResourcePods:             resource.MustParse("110"),
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse(gpu),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse(cpu),
				corev1.ResourceMemory:           resource.MustParse(memory),
				corev1.ResourcePods:             resource.MustParse("110"),
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse(gpu),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
}

func makePod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-test-pod",
			UID:       types.UID("gpu-test-pod-uid"),
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			SchedulerName: "default-scheduler",
			Containers: []corev1.Container{
				{
					Name:  "gpu-container",
					Image: "busybox",
					Command: []string{"sleep", "3600"},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:              resource.MustParse("2"),
							corev1.ResourceMemory:           resource.MustParse("4Gi"),
							corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
						},
					},
				},
			},
		},
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create two virtual Kubernetes Nodes.
	nodeA := makeNode("node-a", "8", "32Gi", "1")
	nodeB := makeNode("node-b", "16", "64Gi", "4")

	// Fake Kubernetes API client, preloaded with Nodes.
	client := fake.NewSimpleClientset(nodeA, nodeB)

	// Informer factories.
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(
		dynamicClient, 0,
	)

	// Event broadcaster and scheduler recorder.
	broadcaster := events.NewBroadcaster(
		&events.EventSinkImpl{Interface: client.EventsV1()},
	)
	broadcaster.StartRecordingToSink(ctx.Done())

	recorderFactory := profile.NewRecorderFactory(broadcaster)

	// Construct the actual Kubernetes scheduler.
	sched, err := scheduler.New(
		ctx,
		client,
		informerFactory,
		dynamicInformerFactory,
		recorderFactory,
	)
	if err != nil {
		panic(fmt.Errorf("creating scheduler: %w", err))
	}

	// Start informers and wait for their caches to synchronize.
	informerFactory.Start(ctx.Done())
	dynamicInformerFactory.Start(ctx.Done())

	if err := sched.WaitForHandlersSync(ctx); err != nil {
		panic(fmt.Errorf("waiting for informer handlers: %w", err))
	}

	// Start the scheduler.
	go sched.Run(ctx)

	// Submit a Pod requesting two GPUs.
	pod := makePod()
	_, err = client.CoreV1().Pods("default").Create(
		ctx, pod, metav1.CreateOptions{},
	)
	if err != nil {
		panic(fmt.Errorf("creating test Pod: %w", err))
	}

	fmt.Println("Submitted Pod:", pod.Name)
	fmt.Println("Waiting for Kubernetes scheduling decision...")

	// Observe the fake client's Binding action.
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			fmt.Println("\\nSCHEDULER DIAGNOSTICS")
			fmt.Println("--------------------")

			currentPod, err := client.CoreV1().Pods("default").Get(
				context.Background(), pod.Name, metav1.GetOptions{},
			)
			if err == nil {
				fmt.Printf("Pod UID: %s\\n", currentPod.UID)
				fmt.Printf("Pod phase: %s\\n", currentPod.Status.Phase)
				fmt.Printf("Pod conditions: %+v\\n", currentPod.Status.Conditions)
			}

			fmt.Println("\\nKubernetes events:")
			eventList, err := client.CoreV1().Events("default").List(
				context.Background(), metav1.ListOptions{},
			)
			if err == nil {
				for _, event := range eventList.Items {
					fmt.Printf("%s: %s\\n", event.Reason, event.Message)
				}
			}

			fmt.Println("\\nClient actions:")
			for _, action := range client.Actions() {
				fmt.Printf("%s %s subresource=%s\\n",
					action.GetVerb(), action.GetResource(), action.GetSubresource())
			}

			cancel()
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

				fmt.Println()
				fmt.Println("KUBERNETES SCHEDULING RESULT")
				fmt.Println("----------------------------")
				fmt.Println("Pod:  default/" + binding.Name)
				fmt.Println("Node:", binding.Target.Name)
				fmt.Println("Scheduler: default-scheduler")

				cancel()
				return
			}
		}
	}
}
