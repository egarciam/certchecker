package main

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var c client.Client

func main() {
	patch := []byte(`{"metadata":{"annotations":{"version": "v2"}}}`)
	a := c.Patch(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "name",
		},
	}, client.RawPatch(types.StrategicMergePatchType, patch))

	fmt.Printf("%v", a)
}
