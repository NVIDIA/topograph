/*
 * Copyright 2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package netq

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/pkg/topology"
)

const (
	ComputeURL = "nmx/v1/compute-nodes"
)

type ComputeNode struct {
	Id         string `json:"ID"`
	Name       string `json:"Name"`
	DomainUUID string `json:"DomainUUID"`
}

func (p *Provider) getNvlDomains(ctx context.Context) (topology.DomainMap, *httperr.Error) {
	url, headers, httpErr := p.getComputeUrl()
	if httpErr != nil {
		return nil, httpErr
	}

	klog.V(4).Infof("Fetching %s", url)
	f := getRequestFunc(ctx, "GET", url, headers, nil)
	_, data, err := httpreq.DoRequest(f, true)
	if err != nil {
		return nil, err
	}

	return parseComputeNodes(data)
}

func (p *Provider) getComputeUrl() (string, map[string]string, *httperr.Error) {
	auth := p.cred.user + ":" + p.cred.passwd
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	headers := map[string]string{"Authorization": authHeader}

	url, err := httpreq.GetURL(p.params.ApiURL, nil, ComputeURL)
	return url, headers, err
}

func parseComputeNodes(data []byte) (topology.DomainMap, *httperr.Error) {
	var computeNodes []ComputeNode
	err := json.Unmarshal(data, &computeNodes)
	if err != nil {
		return nil, httperr.NewError(http.StatusBadGateway, fmt.Sprintf("nmx output read failed: %v", err))
	}

	domainMap := topology.NewDomainMap()
	for _, node := range computeNodes {
		klog.V(4).Infof("Add NVL domain %q for node %q", node.DomainUUID, node.Name)
		domainMap.AddHost(node.DomainUUID, node.Name, node.Name)
	}

	return domainMap, nil
}
