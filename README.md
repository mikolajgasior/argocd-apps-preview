# argocd-apps-preview
A lightweight ArgoCD utility that generates a preview for every “app‑of‑apps” application, and computes diffs between two sets of manifests.

## Prerequisites

The following tools must be installed:
* kind
* argocd cli
* kubectl

## Running

```
rm -rf outputs
mkdir outputs
go run cmd/apps/*.go
```
