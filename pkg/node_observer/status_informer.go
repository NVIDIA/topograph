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
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/NVIDIA/topograph/internal/httperr"
	"github.com/NVIDIA/topograph/internal/httpreq"
	"github.com/NVIDIA/topograph/internal/k8s"
)

type StatusInformer struct {
	ctx         context.Context
	client      kubernetes.Interface
	nodeFactory informers.SharedInformerFactory
	podFactory  informers.SharedInformerFactory
	reqFunc     httpreq.RequestFunc
	reqExecFunc func(httpreq.RequestFunc, bool) ([]byte, *httperr.Error)
	retryDelay  time.Duration
	timer       *time.Timer
	queue       chan struct{}
	stopCh      chan struct{}
}

func NewStatusInformer(ctx context.Context, client kubernetes.Interface, trigger *Trigger, retryDelay time.Duration, reqFunc httpreq.RequestFunc) (*StatusInformer, error) {
	klog.InfoS("Configuring status informer", "trigger", trigger)

	statusInformer := &StatusInformer{
		ctx:         ctx,
		client:      client,
		retryDelay:  retryDelay,
		reqFunc:     reqFunc,
		reqExecFunc: httpreq.DoRequestWithRetries,
		queue:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
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

	if err := s.startNodeInformer(); err != nil {
		return err
	}

	if err := s.startPodInformer(); err != nil {
		return err
	}

	go s.run()

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
	close(s.stopCh)
}

func (s *StatusInformer) startNodeInformer() error {
	if s.nodeFactory != nil {
		informer := s.nodeFactory.Core().V1().Nodes().Informer()
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				if node, ok := obj.(*corev1.Node); ok {
					klog.V(4).Infof("Informer added node %s", node.Name)
					s.sendRequest()
				}
			},
			//UpdateFunc: func(_, obj any) {} // TODO: clarify the change in node that would require topology update
			DeleteFunc: func(obj any) {
				switch v := obj.(type) {
				case *corev1.Node:
					klog.V(4).Infof("Informer deleted node %s", v.Name)
					s.sendRequest()
				case cache.DeletedFinalStateUnknown:
					if node, ok := v.Obj.(*corev1.Node); ok {
						klog.V(4).Infof("Informer deleted node %s", node.Name)
						s.sendRequest()
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
						s.sendRequest()
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
					s.sendRequest()
				}
			},
			DeleteFunc: func(obj any) {
				switch v := obj.(type) {
				case *corev1.Pod:
					klog.V(4).Infof("Informer deleted pod %s/%s", v.Namespace, v.Name)
					s.sendRequest()
				case cache.DeletedFinalStateUnknown:
					if pod, ok := v.Obj.(*corev1.Pod); ok {
						klog.V(4).Infof("Informer deleted pod %s/%s", pod.Namespace, pod.Name)
						s.sendRequest()
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

func (s *StatusInformer) sendRequest() {
	select {
	case s.queue <- struct{}{}:
	default:
		// Drop if already queued (prevents flooding)
	}
}

func (s *StatusInformer) run() {
	for {
		select {
		case <-s.stopCh:
			return

		case <-s.queue:
			// Cancel any pending retry
			if s.timer != nil {
				s.timer.Stop()
				s.timer = nil
			}
			s.process()

		case <-func() <-chan time.Time {
			if s.timer != nil {
				return s.timer.C
			}
			return nil
		}():
			s.process()
		}
	}
}

func (s *StatusInformer) process() {
	if _, err := s.reqExecFunc(s.reqFunc, false); err != nil {
		klog.Errorf("failed to send HTTP request; retrying in %s: %v", s.retryDelay, err)

		// Reset retry timer
		if s.timer != nil {
			s.timer.Stop()
		}
		s.timer = time.NewTimer(s.retryDelay)
		return
	}

	// clear retry timer
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}
