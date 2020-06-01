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
	"context"
	"fmt"
	"net"
	"time"

	porterapi "github.com/kubesphere/porter/api/v1alpha1"
	porterconst "github.com/kubesphere/porter/pkg/constant"
	"github.com/onsi/ginkgo"
	gobgpapi "github.com/osrg/gobgp/api"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	clientset "k8s.io/client-go/kubernetes"
	commonutils "k8s.io/kubernetes/test/e2e/common"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/auth"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2eservice "k8s.io/kubernetes/test/e2e/framework/service"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serverStartTimeout = framework.PodStartTimeout + 3*time.Minute
	PorterNamespace    = "porter-testns"
	PorterManagerName  = "porter-manager-5dbb9cd96-845k4"
	GoBgpPort          = 17900
	PorterBgpPort      = 17901
)

func newClient(ctx context.Context, pod *v1.Pod) (gobgpapi.GobgpApiClient, context.CancelFunc, error) {
	grpcOpts := []grpc.DialOption{grpc.WithBlock()}
	grpcOpts = append(grpcOpts, grpc.WithInsecure())
	target := pod.Status.PodIP + ":50051"
	cc, cancel := context.WithTimeout(ctx, time.Second)
	conn, err := grpc.DialContext(cc, target, grpcOpts...)
	if err != nil {
		return nil, cancel, err
	}
	return gobgpapi.NewGobgpApiClient(conn), cancel, nil
}

func addConfForGobgp(bgpClient gobgpapi.GobgpApiClient, pod *v1.Pod) {
	_, err := bgpClient.StartBgp(context.Background(), &gobgpapi.StartBgpRequest{
		Global: &gobgpapi.Global{
			As:               65000,
			RouterId:         pod.Status.PodIP,
			ListenPort:       17900,
			UseMultiplePaths: true,
		},
	})
	framework.ExpectNoError(err)
}

func getFamily(ip string) *gobgpapi.Family {
	family := &gobgpapi.Family{
		Afi:  gobgpapi.Family_AFI_IP,
		Safi: gobgpapi.Family_SAFI_UNICAST,
	}
	if net.ParseIP(ip).To4() == nil {
		family = &gobgpapi.Family{
			Afi:  gobgpapi.Family_AFI_IP6,
			Safi: gobgpapi.Family_SAFI_UNICAST,
		}
	}

	return family
}

var _ = KubesphereDescribe("[Porter:BGP]", func() {
	f := framework.NewDefaultFramework("network")

	var c clientset.Interface
	var porterManagerPod *v1.Pod
	var porterClient client.Client
	var ns string
	var gobgpPod *v1.Pod
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

		//get porter-manager
		porterManagerPod, err = c.CoreV1().Pods(PorterNamespace).Get(context.TODO(), PorterManagerName, metav1.GetOptions{})
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
		ginkgo.By("setup gobgp pod")
		test := "/root/test-framework/e2e/network/doc-yaml/"
		podYaml := readFile(test, "gobgp-pod.yaml")
		nsFlag := fmt.Sprintf("--namespace=%v", ns)
		//setup gobgp pod
		podName := "gobgp"
		framework.RunKubectlOrDieInput(ns, podYaml, "create", "-f", "-", nsFlag)
		err := e2epod.WaitTimeoutForPodReadyInNamespace(c, podName, ns, 30*time.Second)
		framework.ExpectNoError(err)
		gobgpPod, err = c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
		framework.ExpectNoError(err)
		bgpClient, _, err := newClient(context.TODO(), gobgpPod)
		framework.ExpectNoError(err)
		addConfForGobgp(bgpClient, gobgpPod)

		bgpConfName := "test-bgpconf"
		bgpConfPort := PorterBgpPort
		ginkgo.By("add bgpconf")
		bgpxxx := &porterapi.BgpConf{
			ObjectMeta: metav1.ObjectMeta{
				Name: bgpConfName,
			},
			Spec: porterapi.BgpConfSpec{
				As:       65001,
				RouterId: "192.168.0.1",
				Port:     int32(bgpConfPort),
			},
		}
		framework.ExpectNoError(porterClient.Create(context.TODO(), bgpxxx, &client.CreateOptions{}))
		defer func() {
			framework.ExpectNoError(porterClient.Delete(context.TODO(), bgpxxx))
		}()

		ginkgo.By("add bgppeer")
		bgppeer := &porterapi.BgpPeer{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-peer",
			},
			Spec: porterapi.BgpPeerSpec{
				Config: porterapi.NeighborConfig{
					PeerAs:          65000,
					NeighborAddress: gobgpPod.Status.PodIP,
				},
				AddPaths:  porterapi.AddPaths{SendMax: 8},
				Transport: porterapi.Transport{RemotePort: GoBgpPort},
			},
		}
		framework.ExpectNoError(porterClient.Create(context.TODO(), bgppeer))
		defer func() {
			framework.ExpectNoError(porterClient.Delete(context.TODO(), bgppeer))
		}()

		ginkgo.By("add gobgp peer")
		_, err = bgpClient.AddPeer(context.TODO(), &gobgpapi.AddPeerRequest{
			Peer: &gobgpapi.Peer{
				Conf: &gobgpapi.PeerConf{
					NeighborAddress: porterManagerPod.Status.PodIP,
					PeerAs:          65001,
				},
				AfiSafis: []*gobgpapi.AfiSafi{
					&gobgpapi.AfiSafi{
						Config: &gobgpapi.AfiSafiConfig{
							Family:  getFamily(porterManagerPod.Status.PodIP),
							Enabled: true,
						},
						AddPaths: &gobgpapi.AddPaths{
							Config: &gobgpapi.AddPathsConfig{
								Receive: true,
								SendMax: 8,
							},
						},
					},
				},
				Transport: &gobgpapi.Transport{
					RemotePort: uint32(PorterBgpPort),
				},
			}})
		framework.ExpectNoError(err)

		ginkgo.By("add Eip")
		eip := &porterapi.Eip{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-eip",
			},
			Spec: porterapi.EipSpec{
				Address: "192.168.99.0/24",
			},
		}
		framework.ExpectNoError(porterClient.Create(context.TODO(), eip))
		defer func() {
			framework.ExpectNoError(porterClient.Delete(context.TODO(), eip))
		}()

		ginkgo.By("add service")
		tcpJig := e2eservice.NewTestJig(c, ns, "test-service")
		_, err = tcpJig.CreateTCPService(nil)
		framework.ExpectNoError(err)
		_, err = tcpJig.Run(nil)
		framework.ExpectNoError(err)
		_, err = tcpJig.UpdateService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[porterconst.PorterAnnotationKey] = porterconst.PorterAnnotationValue
		})
		framework.ExpectNoError(err)
		tcpservice, err := tcpJig.WaitForLoadBalancer(e2eservice.GetServiceLoadBalancerCreationTimeout(c))
		framework.ExpectNoError(err)
		framework.Logf("ingress %v", tcpservice.Status.LoadBalancer.Ingress)

		//TODO   api validation
	})
})

func readFile(test, file string) string {
	from := filepath.Join(test, file)
	return commonutils.SubstituteImageName(string(testfiles.ReadOrDie(from)))
}
