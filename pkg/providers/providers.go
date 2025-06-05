/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package providers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/component"
	"github.com/NVIDIA/topograph/pkg/topology"
)

type Provider interface {
	GenerateTopologyConfig(ctx context.Context, pageSize *int, instances []topology.ComputeInstances) (*topology.Vertex, error)
}

type Config struct {
	Creds  map[string]string
	Params map[string]any
}
type NamedLoader = component.NamedLoader[Provider, Config]
type Loader = component.Loader[Provider, Config]
type Registry component.Registry[Provider, Config]

var ErrUnsupportedProvider = errors.New("unsupported provider")

func NewRegistry(namedLoaders ...NamedLoader) Registry {
	return Registry(component.NewRegistry(namedLoaders...))
}

func (r Registry) Get(name string) (Loader, error) {
	loader, ok := r[name]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q, %w", name, ErrUnsupportedProvider)
	}

	return loader, nil
}

func HttpReq(method, url string, headers map[string]string) (string, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return "", err
	}
	for key, val := range headers {
		req.Header.Add(key, val)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func ParseInstanceOutput(buff *bytes.Buffer) (map[string]string, error) {
	i2n := map[string]string{}
	scanner := bufio.NewScanner(buff)
	for scanner.Scan() {
		arr := strings.Split(scanner.Text(), ": ")
		if len(arr) == 2 {
			node, instance := arr[0], arr[1]
			klog.V(4).Info("Node name: ", node, "Instance ID: ", instance)
			i2n[instance] = node
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return i2n, nil
}

func ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %v", path, err)
	}

	return string(data), nil
}
