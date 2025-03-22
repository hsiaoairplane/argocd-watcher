package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	toolsWatch "k8s.io/client-go/tools/watch"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-redis/redis/v7"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Define flags for configuration
	redisAddr := flag.String("redis-addr", "localhost:16379", "Redis server address")
	redisDB := flag.Int("redis-db", 1, "Redis database number")
	argocdNamespace := flag.String("argocd-namespace", "argocd", "ArgoCD namespace")

	// Parse command-line flags
	flag.Parse()

	namespace := *argocdNamespace

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:        *redisAddr,
		DB:          *redisDB,
		DialTimeout: 5 * time.Second,
	})

	// Test connection
	pong, err := rdb.Ping().Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Infof("Connected to Redis: %s", pong)

	config := ctrl.GetConfigOrDie()
	dynamicClient := dynamic.NewForConfigOrDie(config)
	resource := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	appList, err := dynamicClient.Resource(resource).Namespace(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for _, item := range appList.Items {
		// Remove the metadata.managedFields field
		unstructured.RemoveNestedField(item.Object, "metadata", "managedFields")

		specProject, _, err := unstructured.NestedString(item.Object, "spec", "project")
		if err != nil {
			log.Errorf("Error getting spec.project: %v", err)
			return
		}

		// Set the key-value pair
		key := fmt.Sprintf("%s|%s", specProject, item.GetName())
		val, _ := json.Marshal(item.Object)

		err = rdb.Set(key, val, time.Hour).Err()
		if err != nil {
			log.Fatalf("Failed to set key: %v", err)
		}
	}

	log.Infoln("Starting watcher...")

	initRV := appList.GetResourceVersion()
	retryWatcher, err := toolsWatch.NewRetryWatcher(initRV, &cache.ListWatch{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.ResourceVersion = initRV
			options.Watch = true
			return dynamicClient.Resource(resource).Namespace(namespace).Watch(context.Background(), options)
		},
	})
	if err != nil {
		log.Fatalf("Failed to create retry watcher: %v", err)
	}
	defer retryWatcher.Stop()

	// SIGTERM handler
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP)

	for {
		select {
		case event := <-retryWatcher.ResultChan():
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				log.Errorln("Failed to cast event object to Unstructured")
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				log.Debugf("Application added/modified: %v", event.Object)

				// Remove the metadata.managedFields field
				unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

				// Print spec.project
				specProject, _, err := unstructured.NestedString(obj.Object, "spec", "project")
				if err != nil {
					log.Debugf("Error getting spec.project: %v", err)
					return
				}

				// Set and Get a key-value pair
				key := fmt.Sprintf("%s|%s", specProject, obj.GetName())
				val, _ := json.Marshal(event.Object)

				err = rdb.Set(key, val, time.Hour).Err()
				if err != nil {
					log.Fatalf("Failed to set key: %v", err)
				}

			case watch.Deleted:
				log.Debugf("Application deleted: %v", event.Object)

				specProject, _, err := unstructured.NestedString(obj.Object, "spec", "project")
				if err != nil {
					log.Errorf("Error getting spec.project: %v", err)
					return
				}
				log.Debugf("spec.project: %s", specProject)

				// Set and Get a key-value pair
				key := fmt.Sprintf("%s|%s", specProject, obj.GetName())
				err = rdb.Del(key).Err()
				if err != nil {
					log.Fatalf("Failed to set key: %v", err)
				}

			case watch.Bookmark, watch.Error:
			default:
			}
		case <-sig:
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}
