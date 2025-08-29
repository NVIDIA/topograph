/*
 * Copyright 2024-2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httpreq"
)

type NodeInformer struct {
	ctx         context.Context
	client      kubernetes.Interface
	reqFunc     httpreq.RequestFunc
	nodeFactory informers.SharedInformerFactory
	podFactory  informers.SharedInformerFactory
}

func NewNodeInformer(ctx context.Context, client kubernetes.Interface, trigger *Trigger, reqFunc httpreq.RequestFunc) (*NodeInformer, error) {
	klog.InfoS("Configuring node informer", "trigger", trigger)

	informer := &NodeInformer{
		ctx:     ctx,
		client:  client,
		reqFunc: reqFunc,
	}

	if len(trigger.NodeSelector) != 0 {
		listOptionsFunc := func(options *metav1.ListOptions) {
			options.LabelSelector = labels.Set(trigger.NodeSelector).AsSelector().String()
		}
		informer.nodeFactory = informers.NewSharedInformerFactoryWithOptions(
			client, 0, informers.WithTweakListOptions(listOptionsFunc))
	}

	if trigger.PodSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(trigger.PodSelector)
		if err != nil {
			return nil, err
		}

		listOptionsFunc := func(options *metav1.ListOptions) {
			options.LabelSelector = selector.String()
		}
		informer.podFactory = informers.NewSharedInformerFactoryWithOptions(
			client, 0, informers.WithTweakListOptions(listOptionsFunc))
	}

	return informer, nil
}

func (n *NodeInformer) Start() error {
	klog.Infof("Starting node informer")

	if n.nodeFactory != nil {
		err := n.startInformer(n.nodeFactory.Core().V1().Nodes().Informer())
		if err != nil {
			return err
		}
	}

	if n.podFactory != nil {
		err := n.startInformer(n.podFactory.Core().V1().Pods().Informer())
		if err != nil {
			return err
		}
	}

	return nil
}

func (n *NodeInformer) Stop(_ error) {
	klog.Infof("Stopping node informer")
	if n.nodeFactory != nil {
		n.nodeFactory.Shutdown()
	}
	if n.podFactory != nil {
		n.podFactory.Shutdown()
	}
}

func (n *NodeInformer) startInformer(informer cache.SharedIndexInformer) error {
	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: n.eventHandler("added"),
		// TODO: clarify the change in pod/node that would require topology update
		//UpdateFunc: func(_, obj any) {}
		DeleteFunc: n.eventHandler("deleted"),
	})
	if err != nil {
		return err
	}

	informer.Run(n.ctx.Done())
	return nil
}

func (n *NodeInformer) eventHandler(action string) func(obj any) {
	return func(obj any) {
		switch v := obj.(type) {
		case *corev1.Pod:
			klog.V(4).Infof("Informer %s pod %s/%s", action, v.Namespace, v.Name)
		case *corev1.Node:
			klog.V(4).Infof("Informer %s node %s", action, v.Name)
		default:
			klog.V(4).Infof("Informer %s %T %v", action, obj, obj)
		}
		n.sendRequest()
	}
}

func (n *NodeInformer) sendRequest() {
	_, _, err := httpreq.DoRequestWithRetries(n.reqFunc)
	if err != nil {
		klog.Errorf("failed to send HTTP request: %v", err)
	}
}
