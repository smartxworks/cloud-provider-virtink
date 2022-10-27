package provider

import (
	"context"
	"fmt"
	"strings"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
	virtclientset "github.com/smartxworks/virtink/pkg/generated/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

type instancesV2 struct {
	namespace  string
	virtClient virtclientset.Interface
}

var _ cloudprovider.InstancesV2 = &instancesV2{}

func (i *instancesV2) InstanceExists(ctx context.Context, node *corev1.Node) (bool, error) {
	vm, err := i.getVM(ctx, node)
	if err != nil {
		return false, fmt.Errorf("get VM: %s", err)
	}
	return vm != nil, nil
}

func (i *instancesV2) InstanceShutdown(ctx context.Context, node *corev1.Node) (bool, error) {
	vm, err := i.getVM(ctx, node)
	if err != nil {
		return false, fmt.Errorf("get VM: %s", err)
	}
	if vm == nil {
		return false, cloudprovider.InstanceNotFound
	}
	return vm.Status.Phase != virtv1alpha1.VirtualMachineRunning, nil
}

func (i *instancesV2) InstanceMetadata(ctx context.Context, node *corev1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.getVM(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("get VM: %s", err)
	}
	if vm == nil {
		return nil, cloudprovider.InstanceNotFound
	}
	return &cloudprovider.InstanceMetadata{
		ProviderID: fmt.Sprintf("virtink://%s", vm.UID),
	}, nil
}

func (i *instancesV2) getVM(ctx context.Context, node *corev1.Node) (*virtv1alpha1.VirtualMachine, error) {
	var vmUID types.UID
	if node.Spec.ProviderID != "" {
		vmUID = types.UID(strings.TrimPrefix(node.Spec.ProviderID, "virtink://"))
	}

	vmList, err := i.virtClient.VirtV1alpha1().VirtualMachines(i.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list VMs: %s", err)
	}
	for _, vm := range vmList.Items {
		if (vmUID != "" && vm.UID == vmUID) || (vmUID == "" && vm.Name == node.Name) {
			return &vm, nil
		}
	}
	return nil, nil
}

type instancesV2Logger struct {
	cloudprovider.InstancesV2
}

func (i *instancesV2Logger) InstanceExists(ctx context.Context, node *corev1.Node) (bool, error) {
	ret, err := i.InstancesV2.InstanceExists(ctx, node)
	klog.V(1).InfoS("InstanceExists", "node", node, "ret", ret, "err", err)
	return ret, err
}

func (i *instancesV2Logger) InstanceShutdown(ctx context.Context, node *corev1.Node) (bool, error) {
	ret, err := i.InstanceShutdown(ctx, node)
	klog.V(1).InfoS("InstanceShutdown", "node", node, "ret", ret, "err", err)
	return ret, err
}

func (i *instancesV2Logger) InstanceMetadata(ctx context.Context, node *corev1.Node) (*cloudprovider.InstanceMetadata, error) {
	ret, err := i.InstancesV2.InstanceMetadata(ctx, node)
	klog.V(1).InfoS("InstanceMetadata", "node", node, "ret", ret, "err", err)
	return ret, err
}
