package provider

import (
	"context"
	"fmt"
	"strings"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type instanceV2 struct {
	client    client.Client
	namespace string
}

func (i *instanceV2) InstanceExists(ctx context.Context, node *corev1.Node) (bool, error) {
	vm, err := i.getVMByNode(ctx, node)
	if err != nil {
		return false, err
	}
	if vm == nil {
		return false, nil
	}
	return true, nil
}

func (i *instanceV2) InstanceShutdown(ctx context.Context, node *corev1.Node) (bool, error) {
	vm, err := i.getVMByNode(ctx, node)
	if err != nil {
		return false, err
	}
	if vm == nil {
		return false, cloudprovider.InstanceNotFound
	}
	// TODO: 1. non-persistent VM is not recoverable, 2: consider VM run strategy impact
	return vm.Status.Phase != virtv1alpha1.VirtualMachineRunning, nil
}

func (i *instanceV2) InstanceMetadata(ctx context.Context, node *corev1.Node) (*cloudprovider.InstanceMetadata, error) {
	vm, err := i.getVMByNode(ctx, node)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, cloudprovider.InstanceNotFound
	}

	var region, zone string
	var infraNode corev1.Node
	if err := i.client.Get(ctx, types.NamespacedName{Name: vm.Status.NodeName}, &infraNode); err != nil {
		return nil, err
	}

	if val, ok := node.Labels[corev1.LabelTopologyRegion]; ok {
		region = val
	}
	if val, ok := node.Labels[corev1.LabelTopologyZone]; ok {
		zone = val
	}

	return &cloudprovider.InstanceMetadata{
		ProviderID: fmt.Sprintf("virtink://%s", vm.UID),
		Region:     region,
		Zone:       zone,
	}, nil
}

func (i *instanceV2) getVMByNode(ctx context.Context, node *corev1.Node) (*virtv1alpha1.VirtualMachine, error) {
	if !strings.HasPrefix(node.Spec.ProviderID, "virtink://") {
		return nil, fmt.Errorf("providerID '%s' didn't match expected format 'virtink://<vm-uid>'", node.Spec.ProviderID)
	}
	vmUID := strings.TrimPrefix(node.Spec.ProviderID, "virtink://")

	var vm virtv1alpha1.VirtualMachine
	vmKey := types.NamespacedName{
		Namespace: i.namespace,
		Name:      node.Name,
	}
	found := true
	if err := i.client.Get(ctx, vmKey, &vm); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("get VM: %s", err)
		}
		found = false
	}
	if found && string(vm.UID) == vmUID {
		return &vm, nil
	}

	var vmList virtv1alpha1.VirtualMachineList
	// TODO: use index
	if err := i.client.List(ctx, &vmList, client.InNamespace(i.namespace)); err != nil {
		return nil, err
	}
	if len(vmList.Items) == 0 {
		return nil, nil
	}
	for i := range vmList.Items {
		if string(vmList.Items[i].UID) == vmUID {
			return vmList.Items[i].DeepCopy(), nil
		}
	}
	return nil, nil
}
