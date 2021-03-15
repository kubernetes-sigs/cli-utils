module sigs.k8s.io/cli-utils

go 1.16

require (
	github.com/go-errors/errors v1.0.1
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/google/uuid v1.1.2
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/onsi/ginkgo v1.14.2
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.1
	github.com/stretchr/testify v1.6.1
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.20.4
	k8s.io/apiextensions-apiserver v0.18.10
	k8s.io/apimachinery v0.20.4
	k8s.io/cli-runtime v0.20.4
	k8s.io/client-go v0.20.4
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.20.4
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/kustomize/kyaml v0.10.14
	sigs.k8s.io/yaml v1.2.0
)
