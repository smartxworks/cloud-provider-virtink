package provider

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type loadBalancer struct {
	client    client.Client
	namespace string
}

func (lb *loadBalancer) GetLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service) (status *corev1.LoadBalancerStatus, exists bool, err error) {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	lbService, err := lb.getLoadBalancerService(ctx, lbName)
	if err != nil {
		klog.Errorf("Failed to get LoadBalancer service: %v", err)
		return nil, false, err
	}
	if lbService == nil {
		return nil, false, nil
	}

	status = &lbService.Status.LoadBalancer
	return status, true, nil
}

func (lb *loadBalancer) GetLoadBalancerName(ctx context.Context, clusterName string, service *corev1.Service) string {
	return fmt.Sprintf("%s-%s-%s", clusterName, service.Namespace, service.Name)
}

func (lb *loadBalancer) EnsureLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) (*corev1.LoadBalancerStatus, error) {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)

	lbService, err := lb.getLoadBalancerService(ctx, lbName)
	if err != nil {
		klog.Errorf("Failed to get LoadBalancer service: %v", err)
		return nil, err
	}

	if lbService != nil {
		if err := lb.UpdateLoadBalancer(ctx, clusterName, service, nodes); err != nil {
			return nil, err
		}
	} else {
		lbService, err = lb.createLoadBalancerService(ctx, clusterName, service)
		if err != nil {
			klog.Errorf("Failed to create LoadBalancer service: %v", err)
			return nil, err
		}
	}

	err = wait.PollImmediate(time.Second, 5*time.Second, func() (bool, error) {
		if len(lbService.Status.LoadBalancer.Ingress) != 0 {
			return true, nil
		}
		var service *corev1.Service
		service, err = lb.getLoadBalancerService(ctx, lbName)
		if err != nil {
			klog.Errorf("Failed to get LoadBalancer service: %v", err)
			return false, err
		}
		if service != nil && len(service.Status.LoadBalancer.Ingress) > 0 {
			lbService = service
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		klog.Errorf("Failed to poll LoadBalancer service: %v", err)
		return nil, err
	}
	return &lbService.Status.LoadBalancer, nil
}

func (lb *loadBalancer) UpdateLoadBalancer(ctx context.Context, clusterName string, service *corev1.Service, nodes []*corev1.Node) error {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	var lbService corev1.Service
	if err := lb.client.Get(ctx, client.ObjectKey{Name: lbName, Namespace: lb.namespace}, &lbService); err != nil {
		if apierrors.IsNotFound(err) {
			klog.Errorf("Service %s doesn't exist in namespace %s: %v", lbName, lb.namespace, err)
			return err
		}
		klog.Errorf("Failed to get Service %s in namespace %s: %v", lbName, lb.namespace, err)
		return err
	}

	ports := generateLoadBalanceServicePorts(service)

	if !equality.Semantic.DeepEqual(ports, lbService.Spec.Ports) {
		lbService.Spec.Ports = ports
		if err := lb.client.Update(ctx, &lbService); err != nil {
			klog.Errorf("Failed to update LoadBalancer service: %v", err)
			return err
		}
	}
	return nil
}

func (lb *loadBalancer) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *corev1.Service) error {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)

	lbService, err := lb.getLoadBalancerService(ctx, lbName)
	if err != nil {
		klog.Errorf("Failed to get LoadBalancer service: %v", err)
		return err
	}
	if lbService != nil {
		if err = lb.client.Delete(ctx, lbService); err != nil {
			klog.Errorf("Failed to delete LoadBalancer service: %v", err)
			return err
		}
	}

	return nil
}

func (lb *loadBalancer) getLoadBalancerService(ctx context.Context, lbName string) (*corev1.Service, error) {
	var service corev1.Service
	if err := lb.client.Get(ctx, types.NamespacedName{Name: lbName, Namespace: lb.namespace}, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &service, nil
}

func generateLoadBalanceServicePorts(service *corev1.Service) []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:     port.Name,
			Protocol: port.Protocol,
			Port:     port.Port,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: port.NodePort,
			},
		})
	}
	return ports
}

func (lb *loadBalancer) createLoadBalancerService(ctx context.Context, clusterName string, service *corev1.Service) (*corev1.Service, error) {
	lbName := lb.GetLoadBalancerName(ctx, clusterName, service)
	ports := generateLoadBalanceServicePorts(service)

	lbService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        lbName,
			Namespace:   lb.namespace,
			Annotations: service.Annotations,
			Labels: map[string]string{
				"cluster.x-k8s.io/nested-service-name":      service.Name,
				"cluster.x-k8s.io/nested-service-namespace": service.Namespace,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"cluster.x-k8s.io/cluster-name": clusterName,
			},
			Type:                  corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy: service.Spec.ExternalTrafficPolicy,
			Ports:                 ports,
		},
	}
	if len(service.Spec.ExternalIPs) > 0 {
		lbService.Spec.ExternalIPs = service.Spec.ExternalIPs
	}
	if service.Spec.LoadBalancerIP != "" {
		lbService.Spec.LoadBalancerIP = service.Spec.LoadBalancerIP
	}
	if service.Spec.HealthCheckNodePort > 0 {
		lbService.Spec.HealthCheckNodePort = service.Spec.HealthCheckNodePort
	}

	if err := lb.client.Create(ctx, lbService); err != nil {
		klog.Errorf("Failed to create load balancer service %s: %v", lbName, err)
		return nil, err
	}
	return lbService, nil
}
