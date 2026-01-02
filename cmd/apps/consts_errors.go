package main

const (
	ExitKindNotFound                  = 101
	ExitArgoCDNotFound                = 102
	ExitKubectlNotFound               = 103
	ExitManifestsDirNotFound          = 104
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
	ErrMsgManifestsDirNotFound          = "manifests directory not found"
	ErrMsgOutputsDirNotFound            = "outputs directory not found"
	ErrMsgOutputsDirNotEmpty            = "outputs directory is not empty"
	ErrMsgCreatingClusterFailed         = "Error creating kind cluster"
	ErrMsgArgoCDInstallationFailed      = "Error installing ArgoCD"
	ErrMsgArgoCDLoggingFailed           = "Error logging into ArgoCD"
	ErrMsgApplyingSecretsFailed         = "Error applying secrets from directory"
	ErrMsgApplyingManifestsFailed       = "Error applying applications from manifests directory"
	ErrMsgRecursivelyApplyingAppsFailed = "Error recursively applying applications"
	ErrMsgDumpingAppManifestsFailed     = "Error dumping application manifests to directory"
)
