package network

import (
	"context"
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/apis"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/executor"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/qingcloud"
	"github.com/yunify/qingcloud-sdk-go/service"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	e2eservice "k8s.io/kubernetes/test/e2e/framework/service"
	e2eskipper "k8s.io/kubernetes/test/e2e/framework/skipper"
	"os"
	"time"
)

const (
	defaultWaitLB = 5 * time.Minute
	defaultConfig = "/etc/qingcloud/lbconfig.yaml"
)

var _ = KubesphereDescribe("[QingCloud:LoadBalance]", func() {
	f := framework.NewDefaultFramework("network")

	klog.SetOutput(ginkgo.GinkgoWriter)

	var (
		c        clientset.Interface
		qcClient executor.QingCloudClientInterface
		qcLB     *qingcloud.QingCloud
		ns       string
	)

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		ns = f.Namespace.Name

		config, err := os.Open(defaultConfig)
		defer config.Close()
		framework.ExpectNoError(err)
		tmp, err := qingcloud.NewQingCloud(config)
		framework.ExpectNoError(err)
		qcLB = tmp.(*qingcloud.QingCloud)

		qcClient = qcLB.Client
	})

	ginkgo.It("Test the encapsulated interface", func() {
		e2eskipper.Skipf("create loadbalance")

		_, err := qcClient.GetSecurityGroupByName(executor.DefaultSgName)
		framework.ExpectNoError(err)
		ginkgo.By("should be tagged")

		ginkgo.By("allocate eip")
		originEips, err := qcClient.GetAvaliableEIPs()
		framework.ExpectNoError(err)
		allocatedEip, err := qcClient.AllocateEIP(nil)
		framework.ExpectNoError(err)
		ginkgo.By("allocated eip should be tagged")
		nowEips, err := qcClient.GetAvaliableEIPs()
		framework.ExpectEqual(len(originEips)+1, len(nowEips))

		found := false
		for _, eip := range nowEips {
			if *eip.Status.EIPID == *allocatedEip.Status.EIPID {
				found = true
			}
		}
		framework.ExpectEqual(found, true)

		ginkgo.By("create lb")
		lb, err := qcClient.CreateLB(&apis.LoadBalancer{
			Spec: apis.LoadBalancerSpec{
				VxNetID:          service.String(qcLB.Config.DefaultVxNetForLB),
				EIPs:             []*string{allocatedEip.Status.EIPID},
				LoadBalancerName: service.String("test-lb"),
			},
		})
		framework.ExpectNoError(err)

		ginkgo.By("get lb by id or name")
		tmp, err := qcClient.GetLoadBalancerByName(*lb.Spec.LoadBalancerName)
		framework.ExpectNoError(err)
		framework.ExpectEqual(*tmp.Status.LoadBalancerID, *lb.Status.LoadBalancerID)

		ginkgo.By("modify lb")
		update := &apis.LoadBalancer{
			Spec: apis.LoadBalancerSpec{
				NodeCount: service.Int(3),
			},
			Status: apis.LoadBalancerStatus{
				LoadBalancerID: lb.Status.LoadBalancerID,
			},
		}
		err = qcClient.ModifyLB(update)
		framework.ExpectNoError(err)

		ginkgo.By("create listener")
		listener, err := qcClient.CreateListener([]*apis.LoadBalancerListener{&apis.LoadBalancerListener{
			Spec: apis.LoadBalancerListenerSpec{
				LoadBalancerID:   lb.Status.LoadBalancerID,
				BackendProtocol:  service.String("tcp"),
				ListenerPort:     service.Int(90),
				ListenerProtocol: service.String("tcp"),
			},
		}})
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(listener), 1)

		ginkgo.By("create backend")
		backend, err := qcClient.CreateBackends([]*apis.LoadBalancerBackend{
			&apis.LoadBalancerBackend{
				Spec: apis.LoadBalancerBackendSpec{
					LoadBalancerListenerID: listener[0].Status.LoadBalancerListenerID,
					ResourceID:             service.String(qcLB.Config.InstanceIDs[0]),
					Port:                   service.Int(99),
				},
			},
		})

		ginkgo.By("update lb")
		err = qcClient.UpdateLB(lb.Status.LoadBalancerID)
		framework.ExpectNoError(err)

		framework.ExpectNoError(err)
		ginkgo.By("delete backend")
		err = qcClient.DeleteBackends(backend)
		framework.ExpectNoError(err)

		ginkgo.By("delete listener")
		err = qcClient.DeleteListener([]*string{listener[0].Status.LoadBalancerListenerID})
		framework.ExpectNoError(err)

		ginkgo.By("delete lb")
		err = qcClient.DeleteLB(lb.Status.LoadBalancerID)
		framework.ExpectNoError(err)

		ginkgo.By("free eip")
		framework.ExpectNoError(qcClient.ReleaseEIP([]*string{allocatedEip.Status.EIPID}))
	})

	ginkgo.It("test parse annotation", func() {
		testSvc := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-svc",
				Namespace: "default-ns",
				UID:       types.UID("hahahahah-hahah"),
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Protocol: v1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 18988,
				},
				},
			},
		}

		_, err := qingcloud.ParseServiceLBConfig(qcLB.Config.ClusterID, testSvc)
		framework.ExpectError(err)

		testSvc.Annotations = map[string]string{
			qingcloud.ServiceAnnotationLoadBalancerNetworkType: qingcloud.NetworkModePublic,
			qingcloud.ServiceAnnotationLoadBalancerEipSource:   qingcloud.UseAvailableOrAllocateOne,
		}
		_, err = qingcloud.ParseServiceLBConfig(qcLB.Config.ClusterID, testSvc)
		framework.ExpectNoError(err)
	})

	ginkgo.It("test qingcloud provider interface", func() {
		e2eskipper.Skipf("haha")

		testSvc := &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-svc",
				Namespace: "default-ns",
				UID:       types.UID("hahahahah-hahah"),
				Annotations: map[string]string{
					qingcloud.ServiceAnnotationLoadBalancerNetworkType: qingcloud.NetworkModePublic,
					qingcloud.ServiceAnnotationLoadBalancerEipSource:   qingcloud.UseAvailableOrAllocateOne,
				},
			},
			Spec: v1.ServiceSpec{
				Ports: []v1.ServicePort{{
					Protocol: v1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 80,
					},
					NodePort: 18988,
				},
				},
			},
		}
		lbconfig, err := qingcloud.ParseServiceLBConfig(qcLB.Config.ClusterID, testSvc)

		_, exists, err := qcLB.GetLoadBalancer(nil, "", testSvc)
		framework.ExpectNoError(err)
		framework.ExpectEqual(exists, false)

		testNode := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: qcLB.Config.InstanceIDs[0],
			},
		}
		lb, err := qcLB.EnsureLoadBalancer(nil, "", testSvc, []*v1.Node{testNode})
		framework.ExpectNoError(err)
		_, exists, err = qcLB.GetLoadBalancer(nil, "", testSvc)
		framework.ExpectNoError(err)
		framework.ExpectEqual(exists, true)
		ginkgo.By("check attribute on lb")
		tmp, err := qcClient.GetLoadBalancerByName(lbconfig.LoadBalancerName)
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(tmp.Status.LoadBalancerListeners), 1)
		framework.ExpectEqual(*tmp.Status.LoadBalancerListeners[0].Spec.ListenerPort, int(testSvc.Spec.Ports[0].Port))
		framework.ExpectEqual(lb.Ingress[0].IP, tmp.Status.VIP[0])

		testNode2 := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: qcLB.Config.InstanceIDs[1],
			},
		}
		err = qcLB.UpdateLoadBalancer(nil, "", testSvc, []*v1.Node{testNode2})
		framework.ExpectNoError(err)
		tmp2, err := qcClient.GetLoadBalancerByName(lbconfig.LoadBalancerName)
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(tmp2.Status.LoadBalancerListeners), 1)
		framework.ExpectEqual(int(*tmp2.Status.LoadBalancerListeners[0].Spec.ListenerPort), int(testSvc.Spec.Ports[0].Port))
		framework.ExpectEqual(*tmp2.Status.LoadBalancerListeners[0].Status.LoadBalancerListenerID, *tmp.Status.LoadBalancerListeners[0].Status.LoadBalancerListenerID, )
		ginkgo.By("check backend change")

		err = qcLB.EnsureLoadBalancerDeleted(nil, "", testSvc)
		framework.ExpectNoError(err)
		_, exists, err = qcLB.GetLoadBalancer(nil, "", testSvc)
		framework.ExpectNoError(err)
		framework.ExpectEqual(exists, false)
	})

	ginkgo.It("test loadbalance service normal lifecycle with e2e", func() {
		tcpJig := e2eservice.NewTestJig(c, ns, "test-service")
		tcpJig.Run(nil)
		_, err := tcpJig.CreateTCPService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[qingcloud.ServiceAnnotationLoadBalancerType] = "0"
			s.Annotations[qingcloud.ServiceAnnotationLoadBalancerNetworkType] = qingcloud.NetworkModeInternal
			s.Annotations[qingcloud.ServiceAnnotationLoadBalancerVxnetID] = qcLB.Config.DefaultVxNetForLB
		})
		tmpSvc, err := tcpJig.WaitForLoadBalancer(defaultWaitLB)
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(tmpSvc.Status.LoadBalancer.Ingress), 1)

		ginkgo.By("Get the id of the load balancer you created, and use it for other services.")
		lbconfig, err := qingcloud.ParseServiceLBConfig(qcLB.Config.ClusterID, tmpSvc)
		outputLB, _ := qcClient.GetLoadBalancerByName(lbconfig.LoadBalancerName)
		framework.ExpectNoError(err)

		tcpJig2 := e2eservice.NewTestJig(c, ns, "test-service2")
		_, err = tcpJig2.CreateTCPService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[qingcloud.ServiceAnnotationLoadBalancerID] = *outputLB.Status.LoadBalancerID
			s.Spec.Ports[0].Port = 81
		})
		tcpJig2.Run(nil)
		framework.ExpectNoError(err)
		tmp2Svc, err := tcpJig2.WaitForLoadBalancer(defaultWaitLB)
		framework.ExpectNoError(err)
		framework.ExpectEqual(len(tmp2Svc.Status.LoadBalancer.Ingress), 1)
		framework.ExpectEqual(tmpSvc.Status.LoadBalancer.Ingress[0].IP, tmp2Svc.Status.LoadBalancer.Ingress[0].IP)

		ginkgo.By("Change service type to NodePort, loadbalance status should be deleted")
		tcpJig2.UpdateService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeNodePort
		})
		_, err = waitForSvcNoIngress(c, tmp2Svc, defaultWaitLB)
		framework.ExpectNoError(err)
	})
})


func waitForSvcNoIngress(client clientset.Interface, svcMeta *v1.Service, timeout time.Duration) (*v1.Service, error) {
	var service *v1.Service

	pollFunc := func() (bool, error) {
		svc, err := client.CoreV1().Services(svcMeta.Namespace).Get(context.TODO(), svcMeta.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			service = svc
			return true, nil
		}
		return false, nil
	}
	if err := wait.PollImmediate(framework.Poll, timeout, pollFunc); err != nil {
		return nil, fmt.Errorf("timed out waiting for service %q to %s", svcMeta.Name, "destroy loadbalance")
	}
	return service, nil
}