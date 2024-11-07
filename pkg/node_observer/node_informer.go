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

package node_observer

import (
	"context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/pkg/utils"
)

type NodeInformer struct {
	ctx     context.Context
	client  kubernetes.Interface
	reqFunc utils.HttpRequestFunc
	factory informers.SharedInformerFactory
}

func NewNodeInformer(ctx context.Context, client kubernetes.Interface, nodeLabels map[string]string, reqFunc utils.HttpRequestFunc) *NodeInformer {
	klog.Infof("Configuring node informer with labels %v", nodeLabels)
	listOptionsFunc := func(options *metav1.ListOptions) {
		options.LabelSelector = labels.Set(nodeLabels).AsSelector().String()
	}
	return &NodeInformer{
		ctx:     ctx,
		client:  client,
		reqFunc: reqFunc,
		factory: informers.NewSharedInformerFactoryWithOptions(client, 0, informers.WithTweakListOptions(listOptionsFunc)),
	}
}

func (n *NodeInformer) Start() error {
	klog.Infof("Starting node informer")

	informer := n.factory.Core().V1().Nodes().Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			klog.V(4).Infof("Node informer added node %s", node.Name)
			n.SendRequest()
		},
		UpdateFunc: func(_, obj interface{}) {
			// TODO: clarify the change in node spec that would warrant topology update
			//n.queue.AddItem(time.Now())
		},
		DeleteFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			klog.V(4).Infof("Node informer deleted node %s", node.Name)
			n.SendRequest()
		},
	})
	if err != nil {
		return err
	}

	informer.Run(n.ctx.Done())

	return nil
}

func (n *NodeInformer) Stop(_ error) {
	klog.Infof("Stopping node informer")
	n.factory.Shutdown()
}

func (n *NodeInformer) SendRequest() {
	_, _, err := utils.HttpRequestWithRetries(n.reqFunc)
	if err != nil {
		klog.Errorf("failed to send HTTP request: %v", err)
	}
}
