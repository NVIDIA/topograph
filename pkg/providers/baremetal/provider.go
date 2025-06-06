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
	NetqLoginUrl string `mapstructure:"netqLoginUrl"`
	NetqApiUrl   string `mapstructure:"netqApiUrl"`
}

type Provider struct {
	pp   ProviderParams
	cred Credentials
}

type Credentials struct {
	Uname string
	Pwd   string
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
	cred, err := GetCred(config.Creds)
	if err != nil {
		return nil, err
	}
	return New(p, cred)
}

func New(params *ProviderParams, cred *Credentials) (*Provider, error) {
	return &Provider{
		pp:   *params,
		cred: *cred,
	}, nil
}

func GetCred(cred map[string]string) (*Credentials, error) {
	if _, ok := cred["uname"]; !ok {
		return nil, fmt.Errorf("error username not provided")
	}

	if _, ok := cred["pwd"]; !ok {
		return nil, fmt.Errorf("error username not provided")
	}

	return &Credentials{Uname: cred["uname"], Pwd: cred["pwd"]}, nil
}

func GetParams(params map[string]any) (*ProviderParams, error) {
	var p ProviderParams
	if err := config.Decode(params, &p); err != nil {
		return nil, fmt.Errorf("error decoding params: %w", err)
	}
	return &p, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(instances) > 1 {
		return nil, ErrMultiRegionNotSupported
	}

	if len(p.pp.NetqLoginUrl) > 0 {
		return generateTopologyConfigForEth(ctx, p.cred, p.pp, instances)
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
