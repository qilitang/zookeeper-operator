# zookeeper-operator
This is a Kubernetes-based Zookeeper operator that supports Zookeeper versions 3.5 and above. It can directly utilize the Zookeeper image available on Docker Hub.

## Getting Started
Certainly, here are the steps to quickly use the Zookeeper operator
### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/crd/bases
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/zookeeper-operator:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/zookeeper-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller from the cluster:

```sh
make undeploy
```


