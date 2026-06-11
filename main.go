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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-redis/redis/v7"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// keyTTL is the expiration applied to every Redis key. The informer resync
// period must stay shorter than this so that unchanged applications are
// re-written before their key expires.
const keyTTL = time.Hour

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

// appKey builds the Redis key for an application: "<spec.project>|<name>".
// A missing spec.project yields an empty project segment; only a type
// mismatch is reported as an error.
func appKey(obj *unstructured.Unstructured) (string, error) {
	project, _, err := unstructured.NestedString(obj.Object, "spec", "project")
	if err != nil {
		return "", fmt.Errorf("getting spec.project: %w", err)
	}
	return fmt.Sprintf("%s|%s", project, obj.GetName()), nil
}

// upsertApp writes the application to Redis under its app key. The object is
// deep-copied before the managedFields are stripped so that the shared
// informer cache is never mutated.
func upsertApp(rdb *redis.Client, obj *unstructured.Unstructured) error {
	obj = obj.DeepCopy()
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")

	key, err := appKey(obj)
	if err != nil {
		return err
	}

	val, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshaling application %q: %w", key, err)
	}

	if err := rdb.Set(key, val, keyTTL).Err(); err != nil {
		return fmt.Errorf("setting key %q: %w", key, err)
	}
	return nil
}

// deleteApp removes the application's key from Redis.
func deleteApp(rdb *redis.Client, obj *unstructured.Unstructured) error {
	key, err := appKey(obj)
	if err != nil {
		return err
	}
	if err := rdb.Del(key).Err(); err != nil {
		return fmt.Errorf("deleting key %q: %w", key, err)
	}
	return nil
}

// toUnstructured extracts an *unstructured.Unstructured from an informer event
// object, transparently unwrapping the tombstone delivered for deletions that
// were missed while the watch was disconnected.
func toUnstructured(obj interface{}) (*unstructured.Unstructured, bool) {
	switch o := obj.(type) {
	case *unstructured.Unstructured:
		return o, true
	case cache.DeletedFinalStateUnknown:
		u, ok := o.Obj.(*unstructured.Unstructured)
		return u, ok
	default:
		return nil, false
	}
}

func main() {
	// Define flags for configuration
	redisAddr := flag.String("redis-addr", "localhost:16379", "Redis server address")
	redisDB := flag.Int("redis-db", 1, "Redis database number")
	argocdNamespace := flag.String("argocd-namespace", "argocd", "ArgoCD namespace")
	metricsPort := flag.String("metrics-port", "8080", "Metrics server port")
	logLevel := flag.String("log-level", "info", "Log level (trace, debug, info, warn, error)")
	resyncPeriod := flag.Duration("resync-period", 30*time.Minute,
		"Informer resync period; must be shorter than the 1h Redis key TTL so keys stay refreshed")

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

	// Handle termination signals and propagate cancellation to the informer.
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

	// A dynamic shared informer handles the initial list, automatically
	// re-lists when the resource version expires (instead of failing with a
	// "too old resource version" error like a bare RetryWatcher), and
	// re-delivers every object once per resync period so the Redis key TTL is
	// kept fresh for applications that never change.
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, *resyncPeriod, namespace, nil)
	informer := factory.ForResource(resource).Informer()

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			u, ok := toUnstructured(obj)
			if !ok {
				log.Errorln("Add: failed to cast event object to Unstructured")
				return
			}
			appEvents.WithLabelValues("ADDED").Inc()
			if err := upsertApp(rdb, u); err != nil {
				log.Errorf("Add: %v", err)
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			u, ok := toUnstructured(newObj)
			if !ok {
				log.Errorln("Update: failed to cast event object to Unstructured")
				return
			}
			appEvents.WithLabelValues("MODIFIED").Inc()
			if err := upsertApp(rdb, u); err != nil {
				log.Errorf("Update: %v", err)
			}
		},
		DeleteFunc: func(obj interface{}) {
			u, ok := toUnstructured(obj)
			if !ok {
				log.Errorln("Delete: failed to cast event object to Unstructured")
				return
			}
			appEvents.WithLabelValues("DELETED").Inc()
			if err := deleteApp(rdb, u); err != nil {
				log.Errorf("Delete: %v", err)
			}
		},
	}); err != nil {
		log.Fatalf("Failed to add event handler: %v", err)
	}

	log.Infoln("Starting watcher...")
	factory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Fatalln("Failed to sync informer cache")
	}
	log.Infoln("Informer cache synced")

	<-ctx.Done()
	log.Infoln("Shutting down watcher...")
}
