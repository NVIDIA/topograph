/*
 * Copyright 2024-2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/internal/k8s"
)

type StatusInformer struct {
	ctx         context.Context
	client      kubernetes.Interface
	reqFunc     httpreq.RequestFunc
	nodeFactory informers.SharedInformerFactory
	podFactory  informers.SharedInformerFactory
	queue       workqueue.TypedRateLimitingInterface[any]
}

func NewStatusInformer(ctx context.Context, client kubernetes.Interface, trigger *Trigger, reqFunc httpreq.RequestFunc) (*StatusInformer, error) {
	klog.InfoS("Configuring status informer", "trigger", trigger)

	statusInformer := &StatusInformer{
		ctx:     ctx,
		client:  client,
		reqFunc: reqFunc,
	}

	if len(trigger.NodeSelector) != 0 {
		listOptionsFunc := func(options *metav1.ListOptions) {
			options.LabelSelector = labels.Set(trigger.NodeSelector).AsSelector().String()
		}
		statusInformer.nodeFactory = informers.NewSharedInformerFactoryWithOptions(
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
		statusInformer.podFactory = informers.NewSharedInformerFactoryWithOptions(
			client, 0, informers.WithTweakListOptions(listOptionsFunc))
	}

	return statusInformer, nil
}

func (s *StatusInformer) Start() error {
	klog.Info("Starting status informer")

	s.queue = workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[any]())

	if err := s.startNodeInformer(); err != nil {
		return err
	}

	if err := s.startPodInformer(); err != nil {
		return err
	}

	go wait.Until(func() {
		for s.processEvent() {
		}
	}, time.Second, s.ctx.Done())

	return nil
}

func (s *StatusInformer) Stop(_ error) {
	klog.Info("Stopping status informer")
	if s.nodeFactory != nil {
		s.nodeFactory.Shutdown()
	}
	if s.podFactory != nil {
		s.podFactory.Shutdown()
	}
	if s.queue != nil {
		s.queue.ShutDown()
	}
}

func (s *StatusInformer) startNodeInformer() error {
	if s.nodeFactory != nil {
		informer := s.nodeFactory.Core().V1().Nodes().Informer()
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if node, ok := obj.(*corev1.Node); ok {
					klog.V(4).Infof("Informer added node %s", node.Name)
					s.queue.Add(struct{}{})
				}
			},
			//UpdateFunc: func(_, obj any) {} // TODO: clarify the change in node that would require topology update
			DeleteFunc: func(obj any) {
				switch v := obj.(type) {
				case *corev1.Node:
					klog.V(4).Infof("Informer deleted node %s", v.Name)
					s.queue.Add(struct{}{})
				case cache.DeletedFinalStateUnknown:
					if node, ok := v.Obj.(*corev1.Node); ok {
						klog.V(4).Infof("Informer deleted node %s", node.Name)
						s.queue.Add(struct{}{})
					}
				}
			},
		})
		if err != nil {
			return err
		}
		s.nodeFactory.Start(s.ctx.Done())
		s.nodeFactory.WaitForCacheSync(s.ctx.Done())
	}
	return nil
}

func (s *StatusInformer) startPodInformer() error {
	if s.podFactory != nil {
		informer := s.podFactory.Core().V1().Pods().Informer()
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if pod, ok := obj.(*corev1.Pod); ok {
					if k8s.IsPodReady(pod) {
						klog.V(4).Infof("Informer added pod %s/%s", pod.Namespace, pod.Name)
						s.queue.Add(struct{}{})
					}
				}
			},
			UpdateFunc: func(oldObj, newObj any) {
				oldPod, ok := oldObj.(*corev1.Pod)
				if !ok {
					return
				}
				newPod, ok := newObj.(*corev1.Pod)
				if !ok {
					return
				}
				if k8s.IsPodReady(oldPod) != k8s.IsPodReady(newPod) {
					klog.V(4).Infof("Informer updated pod %s/%s", newPod.Namespace, newPod.Name)
					s.queue.Add(struct{}{})
				}
			},
			DeleteFunc: func(obj any) {
				switch v := obj.(type) {
				case *corev1.Pod:
					klog.V(4).Infof("Informer deleted pod %s/%s", v.Namespace, v.Name)
					s.queue.Add(struct{}{})
				case cache.DeletedFinalStateUnknown:
					if pod, ok := v.Obj.(*corev1.Pod); ok {
						klog.V(4).Infof("Informer deleted pod %s/%s", pod.Namespace, pod.Name)
						s.queue.Add(struct{}{})
					}
				}
			},
		})
		if err != nil {
			return err
		}
		s.podFactory.Start(s.ctx.Done())
		s.podFactory.WaitForCacheSync(s.ctx.Done())
	}
	return nil
}

func (s *StatusInformer) processEvent() bool {
	item, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(item)

	_, _, err := httpreq.DoRequestWithRetries(s.reqFunc, false)
	if err != nil {
		klog.Errorf("failed to send HTTP request: %v", err)
	}
	s.queue.Forget(item)
	return true
}
