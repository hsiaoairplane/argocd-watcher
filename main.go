package main

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
)

func GetResourcesDynamically(dynamic dynamic.Interface, ctx context.Context, group string, version string, resource string, namespace string) ([]unstructured.Unstructured, error) {
	resourceId := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}
	list, err := dynamic.Resource(resourceId).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func main() {
	ctx := context.Background()
	config := ctrl.GetConfigOrDie()
	dynamic := dynamic.NewForConfigOrDie(config)
	namespace := "argocd"
	items, err := GetResourcesDynamically(dynamic, ctx, "argoproj.io", "v1alpha1", "applications", namespace)
	if err != nil {
		fmt.Println(err)
	} else {
		for _, item := range items {
			// Convert object to raw JSON
			var rawJson interface{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &rawJson)
			if err != nil {
				fmt.Printf("Error converting object to raw JSON: %v\n", err)
				return
			}

			fmt.Printf("Kind: %s, Name: %s/%s\n", item.GetKind(), item.GetName(), item.GetNamespace())

			// print spec
			spec, found, err := unstructured.NestedMap(item.Object, "spec")
			if err != nil {
				fmt.Printf("Error getting spec: %v\n", err)
				return
			}
			if found {
				fmt.Printf("Spec: %+v\n", spec)
			}

			// print spec.namespace
			specNamespace, found, err := unstructured.NestedString(item.Object, "spec.namespace")
			if err != nil {
				fmt.Printf("Error getting spec.namespace: %v\n", err)
				return
			}
			if found {
				fmt.Printf("Spec.Namespace: %s\n", specNamespace)
			}

			// print spec.server
			specServer, found, err := unstructured.NestedString(item.Object, "spec.server")
			if err != nil {
				fmt.Printf("Error getting spec.server: %v\n", err)
				return
			}
			if found {
				fmt.Printf("Spec.Server: %s\n", specServer)
			}
		}
	}
}
