package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// discoverExternalCerts starts the node certificate checker on control-plane
// nodes. This feature is experimental and requires the node checker image.
func (r *CertificateMonitorReconciler) discoverExternalCerts(ctx context.Context) error {
	log := log.FromContext(ctx)

	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList); err != nil {
		log.Error(err, "failed to list nodes")
		return err
	}

	for _, node := range nodeList.Items {
		if !isControlPlaneNode(node) {
			continue
		}

		pod := r.createExternalNodeCheckerPod(node.Name)
		if err := r.Create(ctx, pod); err != nil && !errors.IsAlreadyExists(err) {
			log.Error(err, "failed to create certificate checker pod", "node", node.Name)
		}
	}

	return nil
}

// isControlPlaneNode checks if a node is a control plane node.
func isControlPlaneNode(node corev1.Node) bool {
	labels := node.Labels
	_, isControlPlane := labels["node-role.kubernetes.io/control-plane"]
	_, isMaster := labels["node-role.kubernetes.io/master"]
	return isControlPlane || isMaster
}
