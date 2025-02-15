# argocd-watcher

An **ArgoCD application watcher** monitors the CRUD (Create, Read, Update, Delete) operations of ArgoCD Application custom resources (CRs) and saves the events to Redis for further processing.

## Key Functionality

- **Watch CRUD Operations**: Detect changes to ArgoCD Application CRs in real-time.
- **Redis Integration**: Save event data into a Redis database with the Redis key `<appproject-name>|<application-name>`, value: `<application-cr>`.

## Use Cases

Integrates with [argocd-proxy](https://github.com/hsiaoairplane/argocd-proxy) project to enhance the ArgoCD list application API performance by reading applications from the Redis cache and performing RBAC filtering in-memory using [casbin](https://github.com/casbin/casbin).

## Requirements

- **ArgoCD**: A working ArgoCD setup with applications.
- **Kubernetes**: A Kubernetes cluster where ArgoCD is deployed.
- **Redis**: A running Redis instance to store event data.
- **Go**: Installed Go environment for building and running the watcher.

## Installation

1. Clone this repository:
   ```console
   git clone git@github.com:hsiaoairplane/argocd-watcher.git
   cd argocd-watcher
   ```

2. Build the watcher:
   ```console
   go build -o argocd-watcher *.go
   ```

3. Deploy to Kubernetes:
   
   - Ensure Redis is running and accessible within your cluster.
   - Apply the necessary RBAC roles and bindings for the watcher to access ArgoCD resources.
   - Create a Kubernetes deployment for the watcher.

   Example deployment:
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: argocd-watcher
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: argocd-watcher
     template:
       metadata:
         labels:
           app: argocd-watcher
       spec:
         containers:
         - name: argocd-watcher
           image: hsiaoairplane/argocd-watcher:latest
   ```

4. Run the watcher locally for testing:
   ```console
   ./argocd-watcher --argocd-namespace=<argocd-namespace> --redis-address=<redis-address> --redis-db=<redis-db-index>
   ```

## Configuration

- **Flags**:
  - `--argocd-namespace`: Namespace to monitor.
  - `--redis-address`: Redis server address.
  - `--redis-db`: Redis DB index.
