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
			fmt.Printf("%+v\n", item)
		}
	}
}
