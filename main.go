package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	appEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "argocd_watcher_events_total",
			Help: "Total number of application events",
		},
		[]string{"event_type"},
	)
)

func init() {
	prometheus.MustRegister(appEvents)
}

func main() {
	// Define flags for configuration
	redisAddr := flag.String("redis-addr", "localhost:16379", "Redis server address")
	redisDB := flag.Int("redis-db", 1, "Redis database number")
	argocdNamespace := flag.String("argocd-namespace", "argocd", "ArgoCD namespace")
	metricsPort := flag.String("metrics-port", "8080", "Metrics server port")
	logLevel := flag.String("log-level", "info", "Log level (trace, debug, info, warn, error)")

	// Parse command-line flags
	flag.Parse()

	// Configure log level so that the debug logs below are actually emitted
	// when requested (logrus defaults to info otherwise).
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Invalid log level %q: %v", *logLevel, err)
	}
	log.SetLevel(level)

	namespace := *argocdNamespace

	// Handle termination signals and propagate cancellation to the API
	// server List/Watch calls below.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	// Start metrics server
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Infof("Starting metrics server on :%s", *metricsPort)
		if err := http.ListenAndServe(":"+*metricsPort, nil); err != nil {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

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

	appList, err := dynamicClient.Resource(resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for _, item := range appList.Items {
		// Remove the metadata.managedFields field
		unstructured.RemoveNestedField(item.Object, "metadata", "managedFields")

		specProject, _, err := unstructured.NestedString(item.Object, "spec", "project")
		if err != nil {
			log.Errorf("Error getting spec.project: %v", err)
			continue
		}

		// Set the key-value pair
		key := fmt.Sprintf("%s|%s", specProject, item.GetName())
		val, _ := json.Marshal(item.Object)

		err = rdb.Set(key, val, time.Hour).Err()
		if err != nil {
			log.Errorf("Failed to set key %q: %v", key, err)
			continue
		}
	}

	log.Infoln("Starting watcher...")

	initRV := appList.GetResourceVersion()
	retryWatcher, err := toolsWatch.NewRetryWatcher(initRV, &cache.ListWatch{
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			// Use the ResourceVersion supplied by RetryWatcher so that a
			// reconnect resumes from the last observed version instead of
			// restarting from initRV (which eventually triggers a
			// "too old resource version" error).
			options.Watch = true
			return dynamicClient.Resource(resource).Namespace(namespace).Watch(ctx, options)
		},
	})
	if err != nil {
		log.Fatalf("Failed to create retry watcher: %v", err)
	}
	defer retryWatcher.Stop()

	for {
		select {
		case event := <-retryWatcher.ResultChan():
			obj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				log.Errorln("Failed to cast event object to Unstructured")
				continue
			}

			appEvents.WithLabelValues(string(event.Type)).Inc()

			switch event.Type {
			case watch.Added, watch.Modified:
				log.Debugf("Application added/modified: %v", event.Object)

				// Remove the metadata.managedFields field
				unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

				// Print spec.project
				specProject, _, err := unstructured.NestedString(obj.Object, "spec", "project")
				if err != nil {
					log.Errorf("Error getting spec.project: %v", err)
					continue
				}

				// Set and Get a key-value pair
				key := fmt.Sprintf("%s|%s", specProject, obj.GetName())
				val, _ := json.Marshal(event.Object)

				err = rdb.Set(key, val, time.Hour).Err()
				if err != nil {
					log.Errorf("Failed to set key %q: %v", key, err)
					continue
				}

			case watch.Deleted:
				log.Debugf("Application deleted: %v", event.Object)

				specProject, _, err := unstructured.NestedString(obj.Object, "spec", "project")
				if err != nil {
					log.Errorf("Error getting spec.project: %v", err)
					continue
				}
				log.Debugf("spec.project: %s", specProject)

				// Set and Get a key-value pair
				key := fmt.Sprintf("%s|%s", specProject, obj.GetName())
				err = rdb.Del(key).Err()
				if err != nil {
					log.Errorf("Failed to delete key %q: %v", key, err)
					continue
				}

			case watch.Error:
				log.Errorf("Watch error event: %v", event.Object)

			case watch.Bookmark:
			default:
			}
		case <-ctx.Done():
			log.Infoln("Shutting down watcher...")
			return
		}
	}
}
