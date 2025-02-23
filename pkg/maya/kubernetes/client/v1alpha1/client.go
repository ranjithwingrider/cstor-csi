/*
Copyright 2019 The OpenEBS Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"strings"

	env "github.com/openebs/cstor-csi/pkg/maya/env/v1alpha1"
	"github.com/pkg/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// K8sMasterIPEnvironmentKey is the environment variable key used to
	// determine the kubernetes master IP address
	K8sMasterIPEnvironmentKey env.ENVKey = "OPENEBS_IO_K8S_MASTER"

	// KubeConfigEnvironmentKey is the environment variable key used to
	// determine the kubernetes config
	KubeConfigEnvironmentKey env.ENVKey = "OPENEBS_IO_KUBE_CONFIG"
)

// getInClusterConfigFunc abstracts the logic to get
// kubernetes incluster config
//
// NOTE:
//  typed function makes it simple to mock
type getInClusterConfigFunc func() (*rest.Config, error)

// buildConfigFromFlagsFunc provides the abstraction to get
// kubernetes config from provided flags
//
// NOTE:
//  typed function makes it simple to mock
type buildConfigFromFlagsFunc func(string, string) (*rest.Config, error)

// GetConfigFunc provides the abstraction to get
// kubernetes config from provided client instance
//
// NOTE:
//  typed function makes it simple to mock
type GetConfigFunc func(*Client) (*rest.Config, error)

// GetConfig returns kubernetes config instance
//
// NOTE:
//  This is an implementation of GetConfigFunc
func GetConfig(c *Client) (*rest.Config, error) {
	if c == nil {
		return nil, errors.New("failed to get kubernetes config: nil client was provided")
	}
	return c.GetConfigForPathOrDirect()
}

// getKubeMasterIPFunc provides the abstraction to get
// kubernetes master IP address
//
// NOTE:
//  typed function makes it simple to mock
type getKubeMasterIPFunc func(env.ENVKey) string

// getKubeConfigPathFunc provides the abstraction to get
// kubernetes config path
//
// NOTE:
//  typed function makes it simple to mock
type getKubeConfigPathFunc func(env.ENVKey) string

// getKubernetesDynamicClientFunc provides the abstraction to get
// dynamic kubernetes clientset
//
// NOTE:
//  typed function makes it simple to mock
type getKubernetesDynamicClientFunc func(*rest.Config) (dynamic.Interface, error)

// getKubernetesClientsetFunc provides the abstraction to get
// kubernetes clientset
//
// NOTE:
//  typed function makes it simple to mock
type getKubernetesClientsetFunc func(*rest.Config) (*kubernetes.Clientset, error)

// Client provides common kuberenetes client operations
type Client struct {
	IsInCluster    bool   // flag to let client point to its own cluster
	KubeConfigPath string // kubeconfig path to get kubernetes clientset

	// Below functions are useful during mock

	// handle to get in cluster config
	getInClusterConfig getInClusterConfigFunc

	// handle to get desired kubernetes config
	buildConfigFromFlags buildConfigFromFlagsFunc

	// handle to get kubernetes clienset
	getKubernetesClientset getKubernetesClientsetFunc

	// handle to get dynamic kubernetes clientset
	getKubernetesDynamicClient getKubernetesDynamicClientFunc

	// handle to get kubernetes master IP
	getKubeMasterIP getKubeMasterIPFunc

	// handle to get kubernetes config path
	getKubeConfigPath getKubeConfigPathFunc
}

// OptionFunc is a typed function that abstracts any kind of operation
// against the provided client instance
//
// This is the basic building block to create functional operations
// against the client instance
type OptionFunc func(*Client)

// New returns a new instance of client
func New(opts ...OptionFunc) *Client {
	c := &Client{}
	for _, o := range opts {
		o(c)
	}
	withDefaults(c)
	return c
}

func withDefaults(c *Client) {
	if c.getInClusterConfig == nil {
		c.getInClusterConfig = rest.InClusterConfig
	}
	if c.buildConfigFromFlags == nil {
		c.buildConfigFromFlags = clientcmd.BuildConfigFromFlags
	}
	if c.getKubernetesClientset == nil {
		c.getKubernetesClientset = kubernetes.NewForConfig
	}
	if c.getKubernetesDynamicClient == nil {
		c.getKubernetesDynamicClient = dynamic.NewForConfig
	}
	if c.getKubeMasterIP == nil {
		c.getKubeMasterIP = env.Get
	}
	if c.getKubeConfigPath == nil {
		c.getKubeConfigPath = env.Get
	}
}

// InCluster enables IsInCluster flag
func InCluster() OptionFunc {
	return func(c *Client) {
		c.IsInCluster = true
	}
}

// WithKubeConfigPath sets kubeconfig path
// against this client instance
func WithKubeConfigPath(kubeConfigPath string) OptionFunc {
	return func(c *Client) {
		c.KubeConfigPath = kubeConfigPath
	}
}

// Clientset returns a new instance of kubernetes clientset
func (c *Client) Clientset() (*kubernetes.Clientset, error) {
	config, err := c.GetConfigForPathOrDirect()
	if err != nil {
		return nil, errors.Wrapf(err,
			"failed to get kubernetes clientset: failed to get kubernetes config: IsInCluster {%t}: KubeConfigPath {%s}",
			c.IsInCluster,
			c.KubeConfigPath,
		)
	}
	return c.getKubernetesClientset(config)
}

// Config returns the kubernetes config instance based on available criteria
func (c *Client) Config() (config *rest.Config, err error) {
	// IsInCluster flag holds the top most priority
	if c.IsInCluster {
		return c.getInClusterConfig()
	}

	// ENV holds second priority
	if strings.TrimSpace(c.getKubeMasterIP(K8sMasterIPEnvironmentKey)) != "" ||
		strings.TrimSpace(c.getKubeConfigPath(KubeConfigEnvironmentKey)) != "" {
		return c.getConfigFromENV()
	}

	// Defaults to InClusterConfig
	return c.getInClusterConfig()
}

// ConfigForPath returns the kuberentes config instance based on KubeConfig path
func (c *Client) ConfigForPath(kubeConfigPath string) (config *rest.Config, err error) {
	return c.buildConfigFromFlags("", kubeConfigPath)
}

func (c *Client) GetConfigForPathOrDirect() (config *rest.Config, err error) {
	if c.KubeConfigPath != "" {
		return c.ConfigForPath(c.KubeConfigPath)
	}
	return c.Config()
}

func (c *Client) getConfigFromENV() (config *rest.Config, err error) {
	k8sMaster := c.getKubeMasterIP(K8sMasterIPEnvironmentKey)
	kubeConfig := c.getKubeConfigPath(KubeConfigEnvironmentKey)
	if strings.TrimSpace(k8sMaster) == "" &&
		strings.TrimSpace(kubeConfig) == "" {
		return nil, errors.Errorf(
			"failed to get kubernetes config: missing ENV: atleast one should be set: {%s} or {%s}",
			K8sMasterIPEnvironmentKey,
			KubeConfigEnvironmentKey,
		)
	}
	return c.buildConfigFromFlags(k8sMaster, kubeConfig)
}

// Dynamic returns a kubernetes dynamic client capable of invoking operations
// against kubernetes resources
func (c *Client) Dynamic() (dynamic.Interface, error) {
	config, err := c.GetConfigForPathOrDirect()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get dynamic client")
	}
	return c.getKubernetesDynamicClient(config)
}
