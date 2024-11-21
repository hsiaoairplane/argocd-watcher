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

	appList, err := dynamic.Resource(schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err)
	}

	for _, item := range appList.Items {
		// Convert object to raw JSON
		var rawJson interface{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, &rawJson)
		if err != nil {
			fmt.Printf("Error converting object to raw JSON: %v\n", err)
			return
		}

		fmt.Printf("Kind: %s, Name: %s/%s\n", item.GetKind(), item.GetName(), item.GetNamespace())

		// Print spec.project
		specProject, _, err := unstructured.NestedString(item.Object, "spec", "project")
		if err != nil {
			fmt.Printf("Error getting spec.project: %v\n", err)
			return
		}
		specDestinationNamespace, _, err := unstructured.NestedString(item.Object, "spec", "destination", "namespace")
		if err != nil {
			fmt.Printf("Error getting spec.destination.namespace: %v\n", err)
			return
		}
		specDestinationServer, _, err := unstructured.NestedString(item.Object, "spec", "destination", "server")
		if err != nil {
			fmt.Printf("Error getting spec.destination.server: %v\n", err)
			return
		}
		specDestinationName, _, err := unstructured.NestedString(item.Object, "spec", "destination", "name")
		if err != nil {
			fmt.Printf("Error getting spec.destination.name: %v\n", err)
			return
		}

		fmt.Printf("spec.project: %s\n", specProject)
		fmt.Printf("spec.destination.namespace: %s\n", specDestinationNamespace)
		fmt.Printf("spec.destination.name: %s\n", specDestinationName)
		fmt.Printf("spec.destination.server: %s\n", specDestinationServer)
	}
}
