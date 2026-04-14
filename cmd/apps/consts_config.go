package main

const (
	KindName        = "argocd-app-prev"
	KindImage       = "kindest/node:v1.33.4"
	ArgoCDNamespace = "tools"
	ArgoCDVersion   = "v2.14.11"
	DirManifests    = "manifests"
	DirSecrets      = "secrets"
	DirOutputs      = "outputs"
	DirHooks        = "hooks"
	ArgoCDNodePort  = 30443
)
