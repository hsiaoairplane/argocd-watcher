package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/argoproj/argo-cd/pkg/apiclient"
	"github.com/argoproj/argo-cd/pkg/apiclient/application/clientset/versioned"
	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		conf, exists := os.LookupEnv("KUBECONFIG")
		if exists {
			config, err = clientcmd.BuildConfigFromFlags("", conf)
		} else {
			var homeDir string
			homeDir, err = os.UserHomeDir()
			if err != nil {
				homeDir = ""
			}
			config, err = clientcmd.BuildConfigFromFlags("", filepath.Join(homeDir, ".kube", "config"))
		}
	}
	argoClientset, err := versioned.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	informerFactory := apiclient.NewApplicationClientset(argoClientset).Informers()
	applicationInformer := informerFactory.Argoproj().V1alpha1().Applications().Informer()
	_ = informerFactory.Argoproj().V1alpha1().Applications().Lister()

	applicationEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			application := obj.(*v1alpha1.Application)
			fmt.Printf("New ArgoCD Application Added: %s\n", application.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldApplication := oldObj.(*v1alpha1.Application)
			newApplication := newObj.(*v1alpha1.Application)
			fmt.Printf("ArgoCD Application Updated: %s -> %s\n", oldApplication.Name, newApplication.Name)
		},
		DeleteFunc: func(obj interface{}) {
			application := obj.(*v1alpha1.Application)
			fmt.Printf("ArgoCD Application Deleted: %s\n", application.Name)
		},
	}

	applicationInformer.AddEventHandler(applicationEventHandler)

	informerFactory.Start(context.Background())

	stopCh := make(chan struct{})
	defer close(stopCh)

	// Wait for termination signal to stop the informer
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	// Stop the informer gracefully
	informerFactory.WaitForCacheSync(stopCh)

}
