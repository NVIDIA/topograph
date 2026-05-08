/*
 * Copyright 2026, NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/NVIDIA/topograph/internal/config"
	"github.com/NVIDIA/topograph/internal/files"
	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/pkg/engines"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const NAME = "graph"

type GraphEngine struct {
	params *Params
	cache  engines.InstanceCache
}

type Params struct {
	OutputPath string `mapstructure:"outputPath"`
}

func NamedLoader() (string, engines.Loader) {
	return NAME, Loader
}

func Loader(_ context.Context, params engines.Config) (engines.Engine, *httperr.Error) {

	p, err := getParameters(params)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}

	return &GraphEngine{
		params: p,
	}, nil
}

func getParameters(params engines.Config) (*Params, error) {
	p := &Params{}
	if err := config.Decode(params, p); err != nil {
		return nil, err
	}

	return p, nil
}

func (eng *GraphEngine) GenerateOutput(ctx context.Context, _ *topology.Graph, cis []topology.ComputeInstances, params map[string]any, environment engines.Environment) ([]byte, *httperr.Error) {

	//Get the list of instances from the cache and the list of missing instances
	instancesMap, missing, err := eng.getInstancesFromCache(ctx, cis)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	if len(missing) > 0 {
		//Get the missing instances from the provider either using Pdsh or any other means
		prov, ok := environment.(engines.InstanceProvider)
		if !ok {
			return nil, httperr.NewError(http.StatusInternalServerError, "instance provider not found")
		}
		instances, err := prov.GetInstances(ctx, missing)
		if err != nil {
			return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
		}

		// Add the instances to the instances map and the cache.
		for i := range instances {
			instance := instances[i]
			instancesMap[instance.ID] = instance
			if eng.cache != nil {
				err = eng.cache.Set(ctx, topology.CacheKey(instance.ID), &instances[i])
				if err != nil {
					return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
				}
			}
		}

		// Check that the instance provider returned data for all missing instances.
		//If the instance provider did not return data for all missing instances, return an error
		for _, id := range missing {
			if _, ok := instancesMap[id]; !ok {
				return nil, httperr.NewError(http.StatusInternalServerError,
					fmt.Sprintf("instance provider did not return data for requested instance ID %q", id))
			}
		}
	}

	//Sort the instances map by the instance ID
	keys := slices.Sorted(maps.Keys(instancesMap))

	//Create the instances document
	doc := &topology.Instances{
		Instances: make([]topology.Node, 0, len(keys)),
	}
	for _, key := range keys {
		doc.Instances = append(doc.Instances, instancesMap[key])
	}

	//Marshal the instances document
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	// If no output path is provided, return the data
	if len(eng.params.OutputPath) == 0 {
		return data, nil
	}

	if err := files.Validate(eng.params.OutputPath, "output path"); err != nil {
		return nil, httperr.NewError(http.StatusBadRequest, err.Error())
	}
	if err := files.Create(eng.params.OutputPath, data); err != nil {
		return nil, httperr.NewError(http.StatusInternalServerError, err.Error())
	}

	return []byte("OK\n"), nil
}

func (eng *GraphEngine) GetComputeInstances(ctx context.Context, _ engines.Environment) ([]topology.ComputeInstances, *httperr.Error) {

	//TODO: Implement GraphEngine.GetComputeInstances
	return nil, nil
}

func (eng *GraphEngine) getInstancesFromCache(ctx context.Context, cis []topology.ComputeInstances) (map[string]topology.Node, []string, error) {
	instancesMap := make(map[string]topology.Node)
	missing := []string{}

	for _, ci := range cis {
		for instanceID := range ci.Instances {
			instanceKey := topology.CacheKey(instanceID)
			if eng.cache != nil {
				instance, err := eng.cache.Get(ctx, instanceKey)
				if err != nil {
					return nil, nil, err
				}

				if instance == nil {
					missing = append(missing, instanceID)
					continue
				}
				instancesMap[instanceID] = *instance
			} else {
				missing = append(missing, instanceID)
			}
		}
	}
	return instancesMap, missing, nil
}
