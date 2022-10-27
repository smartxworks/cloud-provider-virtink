package provider

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

type loadBalancer struct {
	namespace  string
	kubeClient kubernetes.Interface
}

var _ cloudprovider.LoadBalancer = &loadBalancer{}

func (l *loadBalancer) GetLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service) (status *corev1.LoadBalancerStatus, exists bool, err error) {
	lbServiceName := l.GetLoadBalancerName(ctx, clusterName, service)
	lbService, err := l.kubeClient.CoreV1().Services(l.namespace).Get(ctx, lbServiceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			lbService = nil
		} else {
			return nil, false, fmt.Errorf("get LB service: %s", err)
		}
	}

	if lbService == nil {
		return nil, false, nil
	} else {
		return &lbService.Status.LoadBalancer, true, nil
	}
}

func (l *loadBalancer) GetLoadBalancerName(ctx context.Context, clusterName string, service *corev1.Service) string {
	return cloudprovider.DefaultLoadBalancerName(service)
}

func (l *loadBalancer) EnsureLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) (*corev1.LoadBalancerStatus, error) {
	lbServiceName := l.GetLoadBalancerName(ctx, clusterName, service)
	lbService, err := l.kubeClient.CoreV1().Services(l.namespace).Get(ctx, lbServiceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			lbService = nil
		} else {
			return nil, fmt.Errorf("get LB service: %s", err)
		}
	}

	if lbService == nil {
		lbService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      lbServiceName,
				Namespace: l.namespace,
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"cluster.x-k8s.io/cluster-name": clusterName,
				},
				Type:                  corev1.ServiceTypeLoadBalancer,
				ExternalTrafficPolicy: service.Spec.ExternalTrafficPolicy,
			},
		}
		setLBServicePorts(lbService, service)

		if len(service.Spec.ExternalIPs) > 0 {
			lbService.Spec.ExternalIPs = service.Spec.ExternalIPs
		}
		if service.Spec.LoadBalancerIP != "" {
			lbService.Spec.LoadBalancerIP = service.Spec.LoadBalancerIP
		}
		if service.Spec.HealthCheckNodePort > 0 {
			lbService.Spec.HealthCheckNodePort = service.Spec.HealthCheckNodePort
		}

		if _, err = l.kubeClient.CoreV1().Services(l.namespace).Create(ctx, lbService, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("create LB service: %s", err)
		}
	} else {
		setLBServicePorts(lbService, service)
		if _, err := l.kubeClient.CoreV1().Services(l.namespace).Update(ctx, lbService, metav1.UpdateOptions{}); err != nil {
			return nil, fmt.Errorf("update LB service: %s", err)
		}
	}

	if err := wait.PollUntil(5*time.Second, func() (bool, error) {
		lbService, err = l.kubeClient.CoreV1().Services(l.namespace).Get(ctx, lbServiceName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("get LB service: %s", err)
		}
		return len(lbService.Status.LoadBalancer.Ingress) > 0, nil
	}, ctx.Done()); err != nil {
		return nil, fmt.Errorf("poll LB service: %s", err)
	}

	return &lbService.Status.LoadBalancer, nil
}

func (l *loadBalancer) UpdateLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) error {
	lbServiceName := l.GetLoadBalancerName(ctx, clusterName, service)
	lbService, err := l.kubeClient.CoreV1().Services(l.namespace).Get(ctx, lbServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get LB service: %s", err)
	}

	setLBServicePorts(lbService, service)
	if _, err := l.kubeClient.CoreV1().Services(l.namespace).Update(ctx, lbService, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update LB service: %s", err)
	}
	return nil
}

func (l *loadBalancer) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *corev1.Service) error {
	lbServiceName := l.GetLoadBalancerName(ctx, clusterName, service)
	if err := l.kubeClient.CoreV1().Services(l.namespace).Delete(ctx, lbServiceName, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete LB service: %s", err)
		}
	}
	return nil
}

func setLBServicePorts(lbService *corev1.Service, service *corev1.Service) {
	var lbServicePorts []corev1.ServicePort
	for _, port := range service.Spec.Ports {
		lbServicePorts = append(lbServicePorts, corev1.ServicePort{
			Name:     port.Name,
			Protocol: port.Protocol,
			Port:     port.Port,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: port.NodePort,
			},
		})
	}
	lbService.Spec.Ports = lbServicePorts
}

type loadBalancerLogger struct {
	cloudprovider.LoadBalancer
}

func (l *loadBalancerLogger) GetLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service) (status *corev1.LoadBalancerStatus, exists bool, err error) {
	ret1, ret2, err := l.LoadBalancer.GetLoadBalancer(ctx, clusterName, service)
	klog.V(1).InfoS("GetLoadBalancer", "clusterName", clusterName, "service", service, "ret1", ret1, "ret2", ret2, "err", err)
	return ret1, ret2, err
}

func (l *loadBalancerLogger) GetLoadBalancerName(ctx context.Context, clusterName string, service *corev1.Service) string {
	ret := l.LoadBalancer.GetLoadBalancerName(ctx, clusterName, service)
	klog.V(1).InfoS("GetLoadBalancerName", "clusterName", clusterName, "service", service, "ret", ret)
	return ret
}

func (l *loadBalancerLogger) EnsureLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) (*corev1.LoadBalancerStatus, error) {
	ret, err := l.LoadBalancer.EnsureLoadBalancer(ctx, clusterName, service, nodes)
	klog.V(1).InfoS("EnsureLoadBalancer", "clusterName", clusterName, "service", service, "nodes", nodes, "ret", ret, "err", err)
	return ret, err
}

func (l *loadBalancerLogger) UpdateLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) error {
	err := l.LoadBalancer.UpdateLoadBalancer(ctx, clusterName, service, nodes)
	klog.V(1).InfoS("UpdateLoadBalancer", "clusterName", clusterName, "service", service, "nodes", nodes, "err", err)
	return err
}

func (l *loadBalancerLogger) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *corev1.Service) error {
	err := l.LoadBalancer.EnsureLoadBalancerDeleted(ctx, clusterName, service)
	klog.V(1).InfoS("EnsureLoadBalancerDeleted", "clusterName", clusterName, "service", service, "err", err)
	return err
}
