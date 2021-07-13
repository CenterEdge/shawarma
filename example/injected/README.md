# Shawarma Sidecar Injection

For larger deployments, it's preferred to inject Shawarma automatically based on annotations.
This simplies deployment pipelines by providing a standarized Shawarma configuration.

## Deploying

1. Ensure that your cluster has [cert-manager](https://cert-manager.io/) installed.
2. `kubectl apply -f .\k8s-sidecar-injector.yaml`
3. Modify `rbac.yaml` for each namespace which will use Shawarma (it is setup for `default`), and apply using `kubectl apply`.

## Usage

To use, simply include a `shawarma.centeredge.io/service-name` annotation on a pod. This annotation should reference the service
which should be monitored to determine application state. [See here for a full list of available annotations](https://github.com/CenterEdge/shawarma-webhook#annotations).

An example pod deployment can be found in (./test-pod.yaml).
