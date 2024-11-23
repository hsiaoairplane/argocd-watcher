package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v7"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	// Redis configuration
	redisAddr := "localhost:16379" // Redis service DNS
	redisPassword := ""            // Set the password if Redis authentication is enabled

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:        redisAddr,
		Password:    redisPassword,
		DB:          1,
		DialTimeout: 5 * time.Second,
	})

	// Test connection
	pong, err := rdb.Ping().Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	fmt.Printf("Connected to Redis: %s\n", pong)

	config := ctrl.GetConfigOrDie()
	dynamic := dynamic.NewForConfigOrDie(config)
	namespace := "argocd"

	appList, err := dynamic.Resource(schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		fmt.Println(err)
	}

	for _, item := range appList.Items {
		// Remove the metadata.managedFields field
		unstructured.RemoveNestedField(item.Object, "metadata", "managedFields")

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

		// Set and Get a key-value pair
		key := fmt.Sprintf("%s|%s|%s|%s|%s", specProject, item.GetName(), specDestinationNamespace, specDestinationServer, specDestinationName)
		val, _ := json.Marshal(item.Object)

		err = rdb.Set(key, val, time.Hour).Err()
		if err != nil {
			log.Fatalf("Failed to set key: %v", err)
		}

		value, err := rdb.Get(key).Result()
		if err != nil {
			log.Fatalf("Failed to get key: %v", err)
		}

		fmt.Printf("Value of %s: %s\n", key, value)
	}
}
