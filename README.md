# argocd-apps-preview
A lightweight Go utility for generating previews and computing diffs for ArgoCD "app-of-apps" configurations, including 
nested Applications and ApplicationSets.

## Status

**⚠️ Work In Progress**

This project is actively under development. Some features are incomplete.

## Expected Features

- 🌳 Handle nested applications and ApplicationSets
- 📋 Generate previews for app-of-apps configurations
- 🔍 Compute diffs between two sets of application manifests (WIP)
- 🔄 Branch switching support (WIP)
- ⚡ Lightweight, single binary distribution

## Motivation

While there are excellent tools available today for previewing ArgoCD changes, most struggle with the complexity of 
"app-of-apps" patterns where applications nest other applications or ApplicationSets.

I designed this tool specifically with **Pull Request workflows** in mind. When reviewing changes that span multiple 
Helm charts and Kustomize overlays, it is often difficult to determine exactly what will be applied to the cluster after
the merge. Existing tools typically stop at the top-level manifest, leaving reviewers blind to the downstream effects of
nested configurations.

This utility bridges that gap by recursively resolving the entire application tree. It provides a clear, accurate diff 
preview of the final manifests that ArgoCD would apply, making code reviews for complex GitOps strategies significantly 
safer and more efficient.

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

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.
