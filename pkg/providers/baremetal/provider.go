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
	params *ProviderParams
	cred   *Credentials
}

type Credentials struct {
	user   string
	passwd string
}

var ErrMultiRegionNotSupported = errors.New("on-prem does not support multi-region topology requests")

func NamedLoader() (string, providers.Loader) {
	return NAME, Loader
}

func Loader(ctx context.Context, config providers.Config) (providers.Provider, error) {
	params, err := GetParams(config.Params)
	if err != nil {
		return nil, err
	}

	var cred *Credentials
	if len(params.NetqApiUrl) != 0 {
		cred, err = GetCred(config.Creds)
		if err != nil {
			return nil, err
		}
	}
	return New(params, cred)
}

func New(params *ProviderParams, cred *Credentials) (*Provider, error) {
	return &Provider{
		params: params,
		cred:   cred,
	}, nil
}

func GetCred(cred map[string]string) (*Credentials, error) {
	user, ok := cred["username"]
	if !ok {
		return nil, fmt.Errorf("username not provided")
	}

	passwd, ok := cred["password"]
	if !ok {
		return nil, fmt.Errorf("password not provided")
	}

	return &Credentials{user: user, passwd: passwd}, nil
}

func GetParams(params map[string]any) (*ProviderParams, error) {
	p := &ProviderParams{}
	if err := config.Decode(params, p); err != nil {
		return nil, fmt.Errorf("failed to decode params: %w", err)
	}
	if len(p.NetqApiUrl) != 0 && len(p.NetqLoginUrl) == 0 {
		return nil, fmt.Errorf("netqLoginUrl is required when netqApiUrl is set")
	}
	if len(p.NetqLoginUrl) != 0 && len(p.NetqApiUrl) == 0 {
		return nil, fmt.Errorf("netqApiUrl is required when netqLoginUrl is set")
	}

	return p, nil
}

func (p *Provider) GenerateTopologyConfig(ctx context.Context, _ *int, instances []topology.ComputeInstances) (*topology.Vertex, error) {
	if len(instances) > 1 {
		return nil, ErrMultiRegionNotSupported
	}

	if len(p.params.NetqLoginUrl) != 0 {
		return generateTopologyConfigForEth(ctx, p.cred, p.params, instances)
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
	return "local", nil
}
