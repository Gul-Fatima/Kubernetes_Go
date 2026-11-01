package energyscore

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	fwk "k8s.io/kube-scheduler/framework"
)

const Name = "EnergyScore"

type Plugin struct{}

var _ fwk.ScorePlugin = &Plugin{}

func New(
	_ context.Context,
	_ runtime.Object,
	_ fwk.Handle,
) (fwk.Plugin, error) {
	return &Plugin{}, nil
}

func (p *Plugin) Name() string {
	return Name
}

func (p *Plugin) Score(
	ctx context.Context,
	state fwk.CycleState,
	pod *corev1.Pod,
	nodeInfo fwk.NodeInfo,
) (int64, *fwk.Status) {
	score, status := scoreNode(pod, nodeInfo)

	if node := nodeInfo.Node(); node != nil {
		klog.InfoS(
			"EnergyScore evaluated node",
			"pod", pod.Namespace+"/"+pod.Name,
			"node", node.Name,
			"score", score,
		)
	}

	return score, status
}

func (p *Plugin) ScoreExtensions() fwk.ScoreExtensions {
	return nil
}
