package provider

import (
	"errors"
	"fmt"
	"io"
	"os"

	virtv1alpha1 "github.com/smartxworks/virtink/pkg/apis/virt/v1alpha1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ProviderName = "virtink"
)

var scheme = runtime.NewScheme()

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, virtinkCloudProviderFactory)
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := virtv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

type cloud struct {
	client client.Client
	config CloudConfig
}

type CloudConfig struct {
	Kubeconfig string `yaml:"kubeconfig"`
	Namespace  string `yaml:"namespace"`
}

func virtinkCloudProviderFactory(config io.Reader) (cloudprovider.Interface, error) {
	if config == nil {
		return nil, fmt.Errorf("no %s cloud provider config file given", ProviderName)
	}

	cloudConfig := CloudConfig{
		Namespace: "default",
	}
	if err := yaml.NewDecoder(config).Decode(&cloudConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cloud provider config: %v", err)
	}
	if cloudConfig.Kubeconfig == "" {
		return nil, errors.New("kubeconfig key is required")
	}

	klog.InfoS("cloud config", "config", cloudConfig)

	infraKubeconfig, err := os.ReadFile(cloudConfig.Kubeconfig)
	if err != nil {
		return nil, err
	}
	clientConfig, err := clientcmd.NewClientConfigFromBytes(infraKubeconfig)
	if err != nil {
		return nil, err
	}
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	return &cloud{
		client: c,
		config: cloudConfig,
	}, nil
}

func (c *cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
}

func (c *cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return &loadBalancer{
		client:    c.client,
		namespace: c.config.Namespace,
	}, true
}

func (c *cloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (c *cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return &instanceV2{
		client:    c.client,
		namespace: c.config.Namespace,
	}, true
}

func (c *cloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

func (c *cloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

func (c *cloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (c *cloud) ProviderName() string {
	return ProviderName
}

func (c *cloud) HasClusterID() bool {
	return true
}
