package baremetal

import (
	"context"
	"errors"
	"fmt"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "baremetal"

type ProviderParams struct {
	NetworkType   string `mapstructure:"network_type"`
	InputFilePath string `mapstructure:"input_file_path"`
}

type Provider struct {
	pp ProviderParams
}

var ErrMultiRegionNotSupported = errors.New("on-prem does not support multi-region topology requests")

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	p, err := GetParams(config.Params)
	if err != nil {
		return nil, err
	}
	return New(p)
}

func New(params *ProviderParams) (*Provider, error) {
	return &Provider{
		pp: *params,
	}, nil
}

func GetParams(params map[string]any) (*ProviderParams, error) {
	var p ProviderParams
	if err := config.Decode(params, &p); err != nil {
		return nil, fmt.Errorf("error decoding params: %w", err)
	}
	if len(p.NetworkType) == 0 {
		return nil, fmt.Errorf("no network type provided for baremetal")
	}

	if len(p.InputFilePath) == 0 {
		return nil, fmt.Errorf("no Netq InputFilePath provided for baremetal")
	}

	return &p, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(instances) > 1 {
		return nil, ErrMultiRegionNotSupported
	}

	if p.pp.NetworkType == "eth" {
		return parseNetq(p.pp.InputFilePath)
	}
	//call mnnvl code from here
	return generateTopologyConfig(ctx, instances)
}

// Instances2NodeMap implements slurm.instanceMapper
func (p *Provider) Instances2NodeMap(ctx context.Context, nodes []string) (map[string]string, error) {
	i2n := make(map[string]string)
	for _, node := range nodes {
		i2n[node] = node
	}

	return i2n, nil
}

// GetComputeInstancesRegion implements slurm.instanceMapper
func (p *Provider) GetComputeInstancesRegion(_ context.Context) (string, error) {
	return "", nil
}
