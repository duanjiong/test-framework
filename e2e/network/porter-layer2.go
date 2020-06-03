package network

import (
	"context"
	"fmt"
	porterapi "github.com/kubesphere/porter/api/v1alpha1"
	"github.com/kubesphere/porter/pkg/constant"
	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/auth"
	e2eservice "k8s.io/kubernetes/test/e2e/framework/service"
	"net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	Layer2Addr = "172.22.0.99-172.22.0.150"
)

var _ = KubesphereDescribe("[Porter:layer2]", func() {
	f := framework.NewDefaultFramework("network")

	var c clientset.Interface
	var ns string
	var porterClient client.Client
	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		ns = f.Namespace.Name

		//config network client
		cfg := f.ClientConfig()
		porterScheme := runtime.NewScheme()
		err := porterapi.AddToScheme(porterScheme)
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

	ginkgo.It("test layer2", func() {
		ginkgo.By("add Eip")
		eip := &porterapi.Eip{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-eip",
			},
			Spec: porterapi.EipSpec{
				Address: Layer2Addr,
				Protocol: constant.PorterProtocolLayer2,
			},
		}
		framework.ExpectNoError(porterClient.Create(context.TODO(), eip))
		defer func() {
			framework.ExpectNoError(porterClient.Delete(context.TODO(), eip))
		}()

		ginkgo.By("add service")
		tcpJig := e2eservice.NewTestJig(c, ns, "test-service")
		_, err := tcpJig.CreateTCPService(nil)
		framework.ExpectNoError(err)
		_, err = tcpJig.Run(tcpJig.AddRCAntiAffinity)
		framework.ExpectNoError(err)
		_, err = tcpJig.UpdateService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[constant.PorterAnnotationKey] = constant.PorterAnnotationValue
			s.Annotations[constant.PorterProtocolAnnotationKey] = constant.PorterProtocolLayer2
		})

		framework.ExpectNoError(err)
		tcpservice, err := tcpJig.WaitForLoadBalancer(30 * time.Second)
		framework.ExpectNoError(err)
		framework.Logf("ingress %v", tcpservice.Status.LoadBalancer.Ingress)

		_, err = net.Dial("tcp4", fmt.Sprintf("%s:%d",tcpservice.Status.LoadBalancer.Ingress[0].IP,tcpservice.Spec.Ports[0].Port))
		framework.ExpectNoError(err)
	})

})