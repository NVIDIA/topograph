/*
 * Copyright 2024-2025 NVIDIA CORPORATION
 * SPDX-License-Identifier: Apache-2.0
 */

package node_observer

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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
	ctx           context.Context
	nodeFactory   informers.SharedInformerFactory
	podFactory    informers.SharedInformerFactory
	apiFactory    informers.SharedInformerFactory
	brokerFactory informers.SharedInformerFactory
	reqFunc       httpreq.RequestFunc
	reqExecFunc   func(httpreq.RequestFunc, bool) ([]byte, *httperr.Error)
	retryDelay    time.Duration
	timer         *time.Timer
	queue         chan struct{}
	stopCh        chan struct{}

	apiServerContainerName string
}

func NewStatusInformer(ctx context.Context, client kubernetes.Interface, trigger *Trigger, apiServer *APIServer, brokerName, brokerNamespace string, retryDelay time.Duration, reqFunc httpreq.RequestFunc) (*StatusInformer, error) {
	klog.InfoS("Configuring status informer", "trigger", trigger, "apiServer", apiServer, "brokerName", brokerName, "brokerNamespace", brokerNamespace)

	statusInformer := &StatusInformer{
		ctx:         ctx,
		retryDelay:  retryDelay,
		reqFunc:     reqFunc,
		reqExecFunc: httpreq.DoRequestWithRetries,
		queue:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
	}

	if trigger != nil && len(trigger.NodeSelector) != 0 {
		listOptionsFunc := func(options *metav1.ListOptions) {
			options.LabelSelector = labels.Set(trigger.NodeSelector).AsSelector().String()
		}
		statusInformer.nodeFactory = informers.NewSharedInformerFactoryWithOptions(
			client, 0, informers.WithTweakListOptions(listOptionsFunc))
	}

	if trigger != nil && trigger.PodSelector != nil {
		podFactory, err := newPodFactory(client, trigger.PodSelector, "")
		if err != nil {
			return nil, err
		}
		statusInformer.podFactory = podFactory
	}

	if apiServer != nil && apiServer.PodSelector != nil {
		podFactory, err := newPodFactory(client, apiServer.PodSelector, apiServer.Namespace)
		if err != nil {
			return nil, err
		}
		statusInformer.apiFactory = podFactory
		statusInformer.apiServerContainerName = apiServer.ContainerName
	}

	if brokerName != "" && brokerNamespace != "" {
		listOptionsFunc := func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", brokerName).String()
		}
		statusInformer.brokerFactory = informers.NewSharedInformerFactoryWithOptions(
			client,
			0,
			informers.WithNamespace(brokerNamespace),
			informers.WithTweakListOptions(listOptionsFunc),
		)
	}

	return statusInformer, nil
}

func newPodFactory(client kubernetes.Interface, selector *metav1.LabelSelector, namespace string) (informers.SharedInformerFactory, error) {
	s, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, err
	}

	listOptionsFunc := func(options *metav1.ListOptions) {
		options.LabelSelector = s.String()
	}

	options := []informers.SharedInformerOption{
		informers.WithTweakListOptions(listOptionsFunc),
	}
	if namespace != "" {
		options = append(options, informers.WithNamespace(namespace))
	}

	return informers.NewSharedInformerFactoryWithOptions(client, 0, options...), nil
}

func (s *StatusInformer) Start() error {
	klog.Info("Starting status informer")

	if err := s.startNodeInformer(); err != nil {
		return err
	}

	if err := s.startPodInformer(); err != nil {
		return err
	}

	if err := s.startAPIServerInformer(); err != nil {
		return err
	}

	if err := s.startBrokerInformer(); err != nil {
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
	if s.apiFactory != nil {
		s.apiFactory.Shutdown()
	}
	if s.brokerFactory != nil {
		s.brokerFactory.Shutdown()
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

func (s *StatusInformer) startAPIServerInformer() error {
	if s.apiFactory != nil {
		informer := s.apiFactory.Core().V1().Pods().Informer()
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return
				}
				if isAPIServerPodReady(pod, s.apiServerContainerName) {
					klog.V(4).Infof("Informer added ready API server pod %s/%s", pod.Namespace, pod.Name)
					s.sendRequest()
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
				if shouldRequestOnAPIServerUpdate(oldPod, newPod, s.apiServerContainerName) {
					klog.V(4).Infof("Informer updated ready API server pod %s/%s", newPod.Namespace, newPod.Name)
					s.sendRequest()
				}
			},
		})
		if err != nil {
			return err
		}
		s.apiFactory.Start(s.ctx.Done())
		s.apiFactory.WaitForCacheSync(s.ctx.Done())
	}
	return nil
}

func (s *StatusInformer) startBrokerInformer() error {
	if s.brokerFactory != nil {
		informer := s.brokerFactory.Apps().V1().DaemonSets().Informer()
		_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				daemonSet, ok := obj.(*appsv1.DaemonSet)
				if !ok {
					return
				}
				klog.V(4).Infof("Informer added node-data-broker DaemonSet %s/%s", daemonSet.Namespace, daemonSet.Name)
				s.sendRequest()
			},
			UpdateFunc: func(oldObj, newObj any) {
				oldDaemonSet, ok := oldObj.(*appsv1.DaemonSet)
				if !ok {
					return
				}
				newDaemonSet, ok := newObj.(*appsv1.DaemonSet)
				if !ok {
					return
				}
				if isBrokerDaemonSetReady(oldDaemonSet) != isBrokerDaemonSetReady(newDaemonSet) ||
					oldDaemonSet.Status.DesiredNumberScheduled != newDaemonSet.Status.DesiredNumberScheduled {
					klog.V(4).Infof("Informer updated node-data-broker DaemonSet %s/%s", newDaemonSet.Namespace, newDaemonSet.Name)
					s.sendRequest()
				}
			},
			DeleteFunc: func(obj any) {
				switch v := obj.(type) {
				case *appsv1.DaemonSet:
					klog.V(4).Infof("Informer deleted node-data-broker DaemonSet %s/%s", v.Namespace, v.Name)
					s.sendRequest()
				case cache.DeletedFinalStateUnknown:
					if daemonSet, ok := v.Obj.(*appsv1.DaemonSet); ok {
						klog.V(4).Infof("Informer deleted node-data-broker DaemonSet %s/%s", daemonSet.Namespace, daemonSet.Name)
						s.sendRequest()
					}
				}
			},
		})
		if err != nil {
			return err
		}
		s.brokerFactory.Start(s.ctx.Done())
		s.brokerFactory.WaitForCacheSync(s.ctx.Done())
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

func shouldRequestOnAPIServerUpdate(oldPod, newPod *corev1.Pod, containerName string) bool {
	return shouldRequestOnWorkloadUpdate(oldPod, newPod, containerName)
}

func shouldRequestOnWorkloadUpdate(oldPod, newPod *corev1.Pod, containerName string) bool {
	if !isWorkloadPodReady(newPod, containerName) {
		return false
	}
	if !isWorkloadPodReady(oldPod, containerName) {
		return true
	}

	oldRestarts, oldFound := containerRestartCount(oldPod, containerName)
	newRestarts, newFound := containerRestartCount(newPod, containerName)
	return oldFound && newFound && newRestarts > oldRestarts
}

func isAPIServerPodReady(pod *corev1.Pod, containerName string) bool {
	return isWorkloadPodReady(pod, containerName)
}

func isWorkloadPodReady(pod *corev1.Pod, containerName string) bool {
	return k8s.IsPodReady(pod) && isContainerRunningAndReady(pod, containerName)
}

func isContainerRunningAndReady(pod *corev1.Pod, containerName string) bool {
	found := false
	for _, status := range pod.Status.ContainerStatuses {
		if containerName != "" && status.Name != containerName {
			continue
		}
		found = true
		if status.Ready && status.State.Running != nil {
			return true
		}
	}
	return containerName == "" && !found
}

func containerRestartCount(pod *corev1.Pod, containerName string) (int32, bool) {
	if containerName != "" {
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == containerName {
				return status.RestartCount, true
			}
		}
		return 0, false
	}

	var restarts int32
	found := false
	for _, status := range pod.Status.ContainerStatuses {
		restarts += status.RestartCount
		found = true
	}
	return restarts, found
}

func (s *StatusInformer) brokerReady() (bool, error) {
	if s.brokerFactory == nil {
		return true, nil
	}

	informer := s.brokerFactory.Apps().V1().DaemonSets().Informer()
	if !informer.HasSynced() {
		return false, nil
	}

	items := informer.GetStore().List()
	if len(items) != 1 {
		return false, nil
	}

	daemonSet, ok := items[0].(*appsv1.DaemonSet)
	if !ok {
		return false, nil
	}
	return brokerDaemonSetReady(daemonSet)
}

func isBrokerDaemonSetReady(daemonSet *appsv1.DaemonSet) bool {
	ready, _ := brokerDaemonSetReady(daemonSet)
	return ready
}

func brokerDaemonSetReady(daemonSet *appsv1.DaemonSet) (bool, error) {
	if daemonSet.Status.DesiredNumberScheduled == 0 {
		return false, fmt.Errorf(
			"node-data-broker DaemonSet %s/%s has 0 desired replicas; check its node selector, affinity, and tolerations",
			daemonSet.Namespace,
			daemonSet.Name,
		)
	}
	return daemonSet.Status.DesiredNumberScheduled == daemonSet.Status.NumberReady, nil
}

func (s *StatusInformer) process() {
	brokerReady, err := s.brokerReady()
	if err != nil {
		klog.Error(err)
	}
	if !brokerReady {
		klog.V(2).Info("Waiting for the node-data-broker DaemonSet to become ready before topology generation")
		if s.timer != nil {
			s.timer.Stop()
		}
		s.timer = time.NewTimer(defaultBrokerRetryDelay)
		return
	}

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
