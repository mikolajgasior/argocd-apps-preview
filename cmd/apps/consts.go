package main

const (
	MaxRecursions                  = 7
	CtxClusterTimeoutSeconds       = 120
	CtxArgoCDTimeoutSeconds        = 360
	SleepSecondsAfterArgoCDInstall = 4
)

const (
	KindName        = "argocd-app-prev"
	KindImage       = "kindest/node:v1.33.4"
	ArgoCDNamespace = "tools"
	ArgoCDVersion   = "v2.14.11"
	ArgoCDNodePort  = 30443
)

const (
	ExitKindNotFound                  = 101
	ExitArgoCDNotFound                = 102
	ExitKubectlNotFound               = 103
	ExitDirNotFound                   = 104
	ExitOutputsDirNotFound            = 105
	ExitOutputsDirNotEmpty            = 106
	ExitCreatingClusterFailed         = 201
	ExitArgoCDInstallationFailed      = 301
	ExitArgoCDLoggingFailed           = 303
	ExitApplyingSecretsFailed         = 304
	ExitApplyingManifestsFailed       = 305
	ExitRecursivelyApplyingAppsFailed = 306
	ExitDumpingAppManifestsFailed     = 307
)

const (
	ErrMsgKindNotFound                  = "kind not found in PATH"
	ErrMsgArgoCDNotFound                = "argocd not found in PATH"
	ErrMsgKubectlNotFound               = "kubectl not found in PATH"
	ErrMsgOutputsDirNotEmpty            = "outputs directory is not empty"
	ErrMsgCreatingClusterFailed         = "Error creating kind cluster"
	ErrMsgArgoCDInstallationFailed      = "Error installing ArgoCD"
	ErrMsgArgoCDLoggingFailed           = "Error logging into ArgoCD"
	ErrMsgApplyingSecretsFailed         = "Error applying secrets from directory"
	ErrMsgApplyingManifestsFailed       = "Error applying applications from manifests directory"
	ErrMsgRecursivelyApplyingAppsFailed = "Error recursively applying applications"
	ErrMsgDumpingAppManifestsFailed     = "Error dumping application manifests to directory"
)
