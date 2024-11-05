// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cluster

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/siderolabs/gen/xslices"
	"github.com/siderolabs/go-talos-support/support"
	"github.com/siderolabs/go-talos-support/support/bundle"
	"github.com/siderolabs/go-talos-support/support/collectors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/provision"
)

// Crashdump creates a support.zip for the cluster.
func Crashdump(ctx context.Context, cluster provision.Cluster, out io.Writer) {
	statePath, err := cluster.StatePath()
	if err != nil {
		fmt.Fprintf(out, "error getting state path: %s\n", err)

		return
	}

	supportZip := filepath.Join(statePath, "support.zip")

	supportFile, err := os.Create(supportZip)
	if err != nil {
		fmt.Fprintf(out, "error creating crashdump file: %s\n", err)

		return
	}

	defer supportFile.Close() //nolint:errcheck

	c, err := client.New(ctx, client.WithDefaultConfig())
	if err != nil {
		fmt.Fprintf(out, "error creating crashdump: %s\n", err)
	}

	nodes := xslices.Map(cluster.Info().Nodes, func(nodeInfo provision.NodeInfo) string {
		return nodeInfo.IPs[0].String()
	})

	controlplane := nodes[0]

	opts := []bundle.Option{
		bundle.WithArchiveOutput(supportFile),
		bundle.WithTalosClient(c),
		bundle.WithNodes(nodes...),
		bundle.WithNumWorkers(1),
	}

	kubeclient, err := getKubernetesClient(ctx, c, controlplane)
	if err == nil {
		opts = append(opts, bundle.WithKubernetesClient(kubeclient))
	}

	options := bundle.NewOptions(opts...)

	collectors, err := collectors.GetForOptions(ctx, options)
	if err != nil {
		fmt.Fprintf(out, "error creating crashdump collector options: %s\n", err)
	}

	if err := support.CreateSupportBundle(ctx, options, collectors...); err != nil {
		fmt.Fprintf(out, "error creating crashdump: %s\n", err)
	}
}

func getKubernetesClient(ctx context.Context, c *client.Client, endpoint string) (*k8s.Clientset, error) {
	kubeconfig, err := c.Kubeconfig(client.WithNodes(ctx, endpoint))
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return nil, err
	}

	restconfig, err := config.ClientConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := k8s.NewForConfig(restconfig)
	if err != nil {
		return nil, err
	}

	// just checking that k8s responds
	_, err = clientset.CoreV1().Namespaces().Get(ctx, "kube-system", v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return clientset, nil
}
