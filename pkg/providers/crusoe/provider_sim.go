/*
 * Copyright 2026 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package crusoe

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/models"
	"github.com/NVIDIA/topograph/pkg/providers"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME_SIM = "crusoe-sim"

type simProvider struct {
	*providers.BaseSimProvider

	nodes []nodeMetadata
}

func NamedLoaderSim() (string, providers.Loader) {
	return NAME_SIM, LoaderSim
}

// LoaderSim builds the provider from a simulation model. Model switch labels
// propagate down to their nodes, so the model supplies the same Crusoe labels a
// live cluster would and the simulation runs the production topology logic.
func LoaderSim(_ context.Context, config providers.Config) (providers.Provider, *httperr.Error) {
	params, err := providers.GetSimulationParams(config.Params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	model, err := models.NewModelFromFile(params.ModelFileName)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest,
			fmt.Sprintf("failed to load model file: %v", err))
	}

	return &simProvider{
		BaseSimProvider: providers.NewBaseSimProvider(model, params.TrimTiers),
		nodes:           nodeMetadataFromModel(model),
	}, nil
}

func (p *simProvider) GenerateTopologyConfig(_ context.Context, _ *int, cis []topology.ComputeInstances) (*topology.Graph, *httperr.Error) {
	requested, httpErr := requestedNodeIDs(cis)
	if httpErr != nil {
		return nil, httpErr
	}

	topo, httpErr := buildClusterTopology(p.nodes, requested)
	if httpErr != nil {
		return nil, httpErr
	}

	return p.ToGraph(NAME_SIM, topo, cis, false), nil
}

// nodeMetadataFromModel returns the model's nodes in a stable order so the
// rendered topology does not churn between runs.
func nodeMetadataFromModel(model *models.Model) []nodeMetadata {
	names := make([]string, 0, len(model.Nodes))
	for name := range model.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)

	nodes := make([]nodeMetadata, 0, len(names))
	for _, name := range names {
		node := model.Nodes[name]
		nodes = append(nodes, nodeMetadata{InstanceID: name, Labels: node.Labels})
	}
	return nodes
}
