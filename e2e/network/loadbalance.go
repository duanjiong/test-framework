package network

import (
	"github.com/onsi/ginkgo"
	"github.com/yunify/qingcloud-cloud-controller-manager/pkg/loadbalance"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	e2eservice "k8s.io/kubernetes/test/e2e/framework/service"
	qcservice "github.com/yunify/qingcloud-sdk-go/service"
)

var _ = KubesphereDescribe("[QingCloud:LoadBalance]", func() {
	f := framework.NewDefaultFramework("network")

	var c clientset.Interface
	var qcClient *QingCloud
	var err error
	var ns string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		ns = f.Namespace.Name
		qcClient, err = newQingCloud()
		framework.ExpectNoError(err)
	})

	ginkgo.It("test create loadbalance", func() {
		tcpJig := e2eservice.NewTestJig(c, ns, "test-service")
		_, err := tcpJig.CreateTCPService(nil)
		tcpJig.Run(nil)
		framework.ExpectNoError(err)
		_, err = tcpJig.UpdateService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[loadbalance.ServiceAnnotationLoadBalancerType] = "0"
			s.Annotations[loadbalance.ServiceAnnotationLoadBalancerEipSource] = string(loadbalance.UseAvailableOrAllocateOne)
		})
	})

	ginkgo.It("test reuse loadbalance", func() {
		ginkgo.By("create eip")
		outputEIP, err := qcClient.eipService.AllocateEIPs(&qcservice.AllocateEIPsInput{
			Bandwidth:   qcservice.Int(100),
			BillingMode: nil,
			Count:       qcservice.Int(1),
			EIPName:     nil,
			NeedICP:     nil,
		})
		framework.ExpectNoError(err)
		defer func() {
			qcClient.eipService.ReleaseEIPs(&qcservice.ReleaseEIPsInput{EIPs:outputEIP.EIPs})
		}()

		ginkgo.By("create loadbalance")
		outputLB, err := qcClient.lbService.CreateLoadBalancer(&qcservice.CreateLoadBalancerInput{
			EIPs:             outputEIP.EIPs,
			HTTPHeaderSize:   nil,
			LoadBalancerName: nil,
			LoadBalancerType: qcservice.Int(0),
			NodeCount:        qcservice.Int(1),
			PrivateIP:        nil,
			ProjectID:        nil,
			SecurityGroup:    nil,
			VxNet:            nil,
		})
		framework.ExpectNoError(err)
		defer func() {
			qcClient.lbService.DeleteLoadBalancers(&qcservice.DeleteLoadBalancersInput{LoadBalancers: []*string{outputLB.LoadBalancerID}})
		}()

		tcpJig := e2eservice.NewTestJig(c, ns, "test-service")
		_, err = tcpJig.CreateTCPService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[loadbalance.ServiceAnnotationLoadBalancerPolicy] = "test-loadbalance"
		})
		framework.ExpectNoError(err)
	})
})
