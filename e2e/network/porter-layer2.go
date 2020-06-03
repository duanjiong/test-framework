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
	Layer2Addr = "172.22.0.100-172.22.0.131"
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
				Address:  Layer2Addr,
				Protocol: constant.PorterProtocolLayer2,
			},
		}
		framework.ExpectNoError(porterClient.Create(context.TODO(), eip))
		defer func() {
			porterClient.Delete(context.TODO(), eip)
		}()

		ginkgo.By("add service")
		for i := 0; i < 10; i++ {
			framework.Logf("create service %d", i)
			tcpJig := e2eservice.NewTestJig(c, ns, "test-service"+fmt.Sprintf("%d", i))
			_, err := tcpJig.CreateTCPService(nil)
			tcpJig.Run(nil)
			framework.ExpectNoError(err)
			_, err = tcpJig.UpdateService(func(s *v1.Service) {
				s.Spec.Type = v1.ServiceTypeLoadBalancer
				if s.ObjectMeta.Annotations == nil {
					s.ObjectMeta.Annotations = map[string]string{}
				}
				s.Annotations[constant.PorterAnnotationKey] = constant.PorterAnnotationValue
				s.Annotations[constant.PorterProtocolAnnotationKey] = constant.PorterProtocolLayer2
				if i==1 {
					ginkgo.By("test special eip in annotation")
					s.Annotations[constant.PorterEIPAnnotationKey] = "172.22.0.103"
				}
				if i==2 {
					ginkgo.By("test special eip in LoadBalancerIP")
					s.Spec.LoadBalancerIP = "172.22.0.101"
				}
			})

			framework.ExpectNoError(err)
			tcpservice, err := tcpJig.WaitForLoadBalancer(30 * time.Second)
			framework.ExpectNoError(err)
			framework.Logf("ingress %v", tcpservice.Status.LoadBalancer.Ingress)

			dest := fmt.Sprintf("%s:%d", tcpservice.Status.LoadBalancer.Ingress[0].IP, tcpservice.Spec.Ports[0].Port)
			framework.Logf("access %v", dest)
			_, err = net.Dial("tcp4", dest)
			framework.ExpectNoError(err)
		}
		ginkgo.By("check eip status")
		time.Sleep(10 *time.Second) //wait eip status sync
		porterClient.Get(context.TODO(), client.ObjectKey{Namespace: eip.Namespace, Name: eip.Name}, eip)
		framework.ExpectEqual(eip.Status, porterapi.EipStatus{
			Occupied: false,
			Usage:    10,
			PoolSize: 32,
		})


		ginkgo.By("check eip protocol")
		eip2 := &porterapi.Eip{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-eip2",
			},
			Spec: porterapi.EipSpec{
				Address:  Layer2Addr,
				Protocol: "xxxx",
			},
		}
		framework.ExpectError(porterClient.Create(context.TODO(), eip2))


		ginkgo.By("change eip address")
		porterClient.Get(context.TODO(), client.ObjectKey{Namespace: eip.Namespace, Name: eip.Name}, eip)
		eip.Spec.Address = "172.22.0.200-172.22.0.250"
		framework.ExpectNoError(porterClient.Update(context.TODO(), eip))
		tcpJig := e2eservice.NewTestJig(c, ns, "test-service"+fmt.Sprintf("%d", 11))
		_, err := tcpJig.CreateTCPService(nil)
		tcpJig.Run(nil)
		framework.ExpectNoError(err)
		_, err = tcpJig.UpdateService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[constant.PorterAnnotationKey] = constant.PorterAnnotationValue
			s.Annotations[constant.PorterProtocolAnnotationKey] = constant.PorterProtocolLayer2
		})
		tcpservice, err := tcpJig.WaitForLoadBalancer(30 * time.Second)
		framework.Logf("test service 11 len %v", tcpservice.Status.LoadBalancer)
		framework.ExpectNoError(err)

		pod := findActivePorterManager(c)
		c.CoreV1().Pods(PorterNamespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		tcpservice, err = tcpJig.WaitForLoadBalancerDestroy("", 0, 120*time.Second)
		framework.ExpectNoError(err)
		framework.Logf("test service 11 %v", tcpservice.Status.LoadBalancer)

		porterClient.Delete(context.TODO(), eip)
	})

})
