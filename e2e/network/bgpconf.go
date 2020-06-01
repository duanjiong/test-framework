/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package network

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	commonutils "k8s.io/kubernetes/test/e2e/common"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/auth"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	porterapi "github.com/duanjiong/network/api/v1alpha1"
)

const (
	serverStartTimeout = framework.PodStartTimeout + 3*time.Minute
)

var _ = KubesphereDescribe("[Porter:BGP]", func() {
	f := framework.NewDefaultFramework("network")

	var c clientset.Interface
	var porterClient client.Client
	var ns string
	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		ns = f.Namespace.Name

		//config network client
		cfg, err := config.GetConfig()
		framework.ExpectNoError(err)
		porterScheme := runtime.NewScheme()
		err = porterapi.AddToScheme(porterScheme)
		framework.ExpectNoError(err)
		porterClient, err = client.New(cfg, client.Options{
			Scheme: porterScheme,
		})
		framework.ExpectNoError(err)

		// this test wants powerful permissions.  Since the namespace names are unique, we can leave this
		// lying around so we don't have to race any caches
		err = auth.BindClusterRoleInNamespace(c.RbacV1(), "edit", f.Namespace.Name,
			rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Namespace: f.Namespace.Name, Name: "default"})
		framework.ExpectNoError(err)

		err = auth.WaitForAuthorizationUpdate(c.AuthorizationV1(),
			serviceaccount.MakeUsername(f.Namespace.Name, "default"),
			f.Namespace.Name, "create", schema.GroupResource{Resource: "pods"}, true)
		framework.ExpectNoError(err)
	})

	framework.ConformanceIt("BgpConf", func() {
			test := "/root/test-framework/e2e/network/doc-yaml/"
			bgpConf := readFile(test, "bgpconf.yaml")
			podYaml := readFile(test, "gobgp-pod.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			podName := "gobgp"
			framework.RunKubectlOrDieInput(ns, bgpConf, "create", "-f", "-")
			framework.RunKubectlOrDieInput(ns, podYaml, "create", "-f", "-", nsFlag)
			err := e2epod.WaitTimeoutForPodReadyInNamespace(c, podName, ns, 30 * time.Second)
			framework.ExpectNoError(err)
			framework.RunKubectlOrDieInput(ns, bgpConf, "delete", "-f", "-")

	})
})

func readFile(test, file string) string {
	from := filepath.Join(test, file)
	return commonutils.SubstituteImageName(string(testfiles.ReadOrDie(from)))
}