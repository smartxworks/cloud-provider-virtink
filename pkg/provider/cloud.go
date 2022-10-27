package provider

import (
	"fmt"
	"io"
	"os"

	virtclientset "github.com/smartxworks/virtink/pkg/generated/clientset/versioned"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
)

func init() {
	cloudprovider.RegisterCloudProvider(ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		var cloudConfig CloudConfig
		if err := yaml.NewDecoder(config).Decode(&cloudConfig); err != nil {
			return nil, fmt.Errorf("unmarshal cloud provider config: %s", err)
		}

		var restConfig *rest.Config
		namespace := cloudConfig.Namespace

		if cloudConfig.KubeconfigPath == "" {
			conf, err := rest.InClusterConfig()
			if err != nil {
				return nil, fmt.Errorf("create in-cluster config: %s", err)
			}

			restConfig = conf
		} else {
			kubeconfig, err := os.ReadFile(cloudConfig.KubeconfigPath)
			if err != nil {
				return nil, fmt.Errorf("read kubeconfig: %s", err)
			}

			clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
			if err != nil {
				return nil, fmt.Errorf("create client config: %s", err)
			}

			conf, err := clientConfig.ClientConfig()
			if err != nil {
				return nil, fmt.Errorf("get client config: %s", err)
			}

			restConfig = conf

			if namespace == "" {
				ns, _, err := clientConfig.Namespace()
				if err != nil {
					return nil, fmt.Errorf("get namespace: %s", err)
				}
				namespace = ns
			}
		}

		kubeClient, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("create Kubernetes client: %s", err)
		}

		virtClient, err := virtclientset.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("create Virtink client: %s", err)
		}

		return &cloud{
			instances: &instancesV2Logger{
				InstancesV2: &instancesV2{
					namespace:  namespace,
					virtClient: virtClient,
				},
			},
			loadBalancer: &loadBalancerLogger{
				LoadBalancer: &loadBalancer{
					namespace:  namespace,
					kubeClient: kubeClient,
				},
			},
		}, nil
	})
}

const (
	ProviderName = "virtink"
)

type CloudConfig struct {
	KubeconfigPath string `yaml:"kubeconfigPath"`
	Namespace      string `yaml:"namespace"`
}

type cloud struct {
	instances    cloudprovider.InstancesV2
	loadBalancer cloudprovider.LoadBalancer
}

var _ cloudprovider.Interface = &cloud{}

func (c *cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
}

func (c *cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return c.loadBalancer, true
}

func (c *cloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

func (c *cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
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
