package baremetal

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/common"
	"github.com/NVIDIA/topograph/pkg/engines/slurm"
)

type Provider struct{}

func GetProvider() (*Provider, error) {
	return &Provider{}, nil
}

func (p *Provider) GetCredentials(_ *common.Credentials) (interface{}, error) {
	return nil, nil
}

func (p *Provider) GetComputeInstances(ctx context.Context, engine common.Engine) ([]common.ComputeInstances, error) {
	klog.InfoS("Getting compute instances", "provider", common.ProviderBM, "engine", engine)

	switch engine.(type) {
	case *slurm.SlurmEngine:
		nodes, err := slurm.GetNodeList(ctx)
		if err != nil {
			return nil, err
		}
		i2n := make(map[string]string)
		for _, node := range nodes {
			i2n[node] = node
		}
		return []common.ComputeInstances{{Instances: i2n}}, nil
	default:
		return nil, fmt.Errorf("unsupported engine %q", engine)
	}
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ interface{}, _ int, instances []common.ComputeInstances) (*common.Vertex, error) {
	if len(instances) > 1 {
		return nil, fmt.Errorf("On-prem does not support multi-region topology requests")
	}

	//call mnnvl code from here
	return generateTopologyConfig(ctx, instances)
}
