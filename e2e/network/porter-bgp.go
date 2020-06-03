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
	"github.com/golang/protobuf/ptypes"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"net"
	"time"

	porterapi "github.com/kubesphere/porter/api/v1alpha1"
	porterconst "github.com/kubesphere/porter/pkg/constant"
	"github.com/onsi/ginkgo"
	gobgpapi "github.com/osrg/gobgp/api"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	PorterNamespace = "porter-testns"
	PorterGrpcPort  = 50051
	GoBgpGrpcPort   = 50052
	GoBgpPort       = 17900
	GoBgpAS         = 65000
	PorterBgpAS     = 65001
	PorterBgpPort   = 17901
	PorterRouterID  = "8.8.8.8"
	defaultTime     = 120
)

type bgpTestGlobal struct {
	as           uint32
	listenPort   int32
	routerID     string
	name         string
	porterClient client.Client
}

func (global *bgpTestGlobal) Create() error {
	bgpxxx := &porterapi.BgpConf{
		ObjectMeta: metav1.ObjectMeta{
			Name: global.name,
		},
		Spec: porterapi.BgpConfSpec{
			As:       global.as,
			RouterId: global.routerID,
			Port:     global.listenPort,
		},
	}

	return global.porterClient.Create(context.TODO(), bgpxxx)
}

func (global *bgpTestGlobal) Delete() {
	bgpxxx := &porterapi.BgpConf{
		ObjectMeta: metav1.ObjectMeta{
			Name: global.name,
		},
	}

	framework.ExpectNoError(global.porterClient.Delete(context.TODO(), bgpxxx))
}

func (global *bgpTestGlobal) Update(listenPort int32) {
	bgpxxx := &porterapi.BgpConf{}

	framework.ExpectNoError(global.porterClient.Get(context.TODO(), client.ObjectKey{Name: global.name}, bgpxxx))

	bgpxxx.Spec.Port = listenPort
	global.listenPort = listenPort

	framework.ExpectNoError(global.porterClient.Update(context.TODO(), bgpxxx))
}

type bgpTestPeer struct {
	address      string
	as           uint32
	port         uint16
	name         string
	porterClient client.Client
	passive      bool
	forward      bool
}

func (peer *bgpTestPeer) Create() {
	bgppeer := &porterapi.BgpPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name: peer.name,
		},
		Spec: porterapi.BgpPeerSpec{
			Config: porterapi.NeighborConfig{
				PeerAs:          peer.as,
				NeighborAddress: peer.address,
			},
			AddPaths: porterapi.AddPaths{SendMax: 8},
			Transport: porterapi.Transport{
				RemotePort:  peer.port,
				PassiveMode: peer.passive,
			},
			UsingPortForward: peer.forward,
		},
	}
	framework.ExpectNoError(peer.porterClient.Create(context.TODO(), bgppeer))
}

func (peer *bgpTestPeer) Delete() {
	bgppeer := &porterapi.BgpPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name: peer.name,
		},
	}
	peer.porterClient.Delete(context.TODO(), bgppeer)
}

func (peer *bgpTestPeer) Update() {
	for i := 0; i < 3; i++ {
		bgppeer := &porterapi.BgpPeer{}
		framework.ExpectNoError(peer.porterClient.Get(context.TODO(), client.ObjectKey{Name: peer.name}, bgppeer))
		framework.Logf("bgppeer %v", bgppeer)
		bgppeer.Spec = porterapi.BgpPeerSpec{
			Config: porterapi.NeighborConfig{
				PeerAs:          peer.as,
				NeighborAddress: peer.address,
			},
			AddPaths: porterapi.AddPaths{SendMax: 8},
			Transport: porterapi.Transport{
				RemotePort:  peer.port,
				PassiveMode: peer.passive,
			},
			UsingPortForward: peer.forward,
		}

		err := peer.porterClient.Update(context.TODO(), bgppeer)
		if err == nil {
			return
		}
		if !apierrors.IsConflict(err) && !apierrors.IsServerTimeout(err) {
			framework.ExpectNoError(err)
		}
	}
}

type gobgpClient struct {
	client gobgpapi.GobgpApiClient
	cancle context.CancelFunc
}

func newGobgpClient(ctx context.Context, pod *v1.Pod, port int) *gobgpClient {
	grpcOpts := []grpc.DialOption{grpc.WithBlock()}
	grpcOpts = append(grpcOpts, grpc.WithInsecure())
	target := pod.Status.PodIP + ":" + fmt.Sprintf("%d", port)
	cc, cancel := context.WithTimeout(ctx, time.Second)
	conn, err := grpc.DialContext(cc, target, grpcOpts...)
	if err != nil {
		return nil
	}
	return &gobgpClient{
		client: gobgpapi.NewGobgpApiClient(conn),
		cancle: cancel,
	}
}

func (client *gobgpClient) addConfForGobgp(pod *v1.Pod) {
	_, err := client.client.StartBgp(context.Background(), &gobgpapi.StartBgpRequest{
		Global: &gobgpapi.Global{
			As:               65000,
			RouterId:         pod.Status.PodIP,
			ListenPort:       17900,
			UseMultiplePaths: true,
			GracefulRestart: &gobgpapi.GracefulRestart{
				Enabled:     true,
				RestartTime: 60,
			},
		},
	})
	framework.ExpectNoError(err)
}

func fromAPIPath(path *gobgpapi.Path) (net.IP, error) {
	for _, attr := range path.Pattrs {
		var value ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(attr, &value); err != nil {
			return nil, fmt.Errorf("failed to unmarshal route distinguisher: %s", err)
		}

		switch a := value.Message.(type) {
		case *gobgpapi.NextHopAttribute:
			nexthop := net.ParseIP(a.NextHop).To4()
			if nexthop == nil {
				if nexthop = net.ParseIP(a.NextHop).To16(); nexthop == nil {
					return nil, fmt.Errorf("invalid nexthop address: %s", a.NextHop)
				}
			}
			return nexthop, nil
		}
	}

	return nil, fmt.Errorf("cannot find nexthop")
}

func (client *gobgpClient) getRoutersForGobgp(ip string) []string {
	listPathRequest := &gobgpapi.ListPathRequest{
		TableType: gobgpapi.TableType_GLOBAL,
		Family:    getFamily(ip),
		Prefixes: []*gobgpapi.TableLookupPrefix{
			&gobgpapi.TableLookupPrefix{
				Prefix: ip,
			},
		},
	}

	var nexthops []string

	responce, err := client.client.ListPath(context.TODO(), listPathRequest)
	framework.ExpectNoError(err)
	dests, err := responce.Recv()
	if err != nil {
		return nil
	}
	for _, path := range dests.Destination.Paths {
		nexthop, _ := fromAPIPath(path)
		nexthops = append(nexthops, nexthop.String())
	}

	return nexthops
}

func (client *gobgpClient) addPeerForGobgp(address string, as uint32, port int) {
	_, err := client.client.AddPeer(context.TODO(), &gobgpapi.AddPeerRequest{
		Peer: &gobgpapi.Peer{
			Conf: &gobgpapi.PeerConf{
				NeighborAddress: address,
				PeerAs:          as,
			},
			AfiSafis: []*gobgpapi.AfiSafi{
				&gobgpapi.AfiSafi{
					Config: &gobgpapi.AfiSafiConfig{
						Family:  getFamily(address),
						Enabled: true,
					},
					AddPaths: &gobgpapi.AddPaths{
						Config: &gobgpapi.AddPathsConfig{
							Receive: true,
							SendMax: 8,
						},
					},
					MpGracefulRestart: &gobgpapi.MpGracefulRestart{
						Config: &gobgpapi.MpGracefulRestartConfig{
							Enabled: true,
						},
					},
				},
			},
			GracefulRestart: &gobgpapi.GracefulRestart{
				Enabled:     true,
				RestartTime: 60,
			},
			Transport: &gobgpapi.Transport{
				RemotePort: uint32(port),
				//PassiveMode: true,
			},
		}})
	framework.ExpectNoError(err)
}

func (client *gobgpClient) deletePeerForGobgp(address string) {
	_, err := client.client.DeletePeer(context.TODO(), &gobgpapi.DeletePeerRequest{
		Address: address,
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

//TODO find master porter-manager, now only support one pod
func findActivePorterManager(c clientset.Interface) *v1.Pod {

	pods, err := c.CoreV1().Pods(PorterNamespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{"app": "porter-manager"}).String(),
	})
	framework.ExpectNoError(err)

	for _, pod := range pods.Items {
		conn, err := net.Dial("tcp", pod.Status.HostIP+":50051")
		if err != nil {
			continue
		} else {
			conn.Close()
			return &pod
		}

	}

	framework.Fail("no active porter-manager")

	return nil
}

var _ = KubesphereDescribe("[Porter:BGP]", func() {
	f := framework.NewDefaultFramework("network")

	var c clientset.Interface
	var porterManagerPod *v1.Pod
	var porterClient client.Client
	var ns string
	var gobgpPod *v1.Pod
	var porterBgpClient *gobgpClient

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
		porterManagerPod = findActivePorterManager(c)
		porterBgpClient = newGobgpClient(context.TODO(), porterManagerPod, PorterGrpcPort)
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
		podYaml := readFile(test, "gobgp-pod.yaml")
		nsFlag := fmt.Sprintf("--namespace=%v", ns)
		//setup gobgp pod
		podName := "gobgp"
		framework.RunKubectlOrDieInput(ns, podYaml, "create", "-f", "-", nsFlag)
		err := e2epod.WaitTimeoutForPodReadyInNamespace(c, podName, ns, 30*time.Second)
		framework.ExpectNoError(err)
		gobgpPod, err = c.CoreV1().Pods(ns).Get(context.TODO(), podName, metav1.GetOptions{})
		framework.ExpectNoError(err)
		bgpClient := newGobgpClient(context.TODO(), gobgpPod, GoBgpGrpcPort)
		framework.ExpectNoError(err)
		bgpClient.addConfForGobgp(gobgpPod)

		ginkgo.By("add Eip")
		eip := &porterapi.Eip{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-eip",
			},
			Spec: porterapi.EipSpec{
				Address: "192.168.99.10-192.168.99.11",
			},
		}
		framework.ExpectNoError(porterClient.Create(context.TODO(), eip))
		defer func() {
			framework.ExpectNoError(porterClient.Delete(context.TODO(), eip))
		}()

		ginkgo.By("add bgppeer")
		bgppeer := &bgpTestPeer{
			address:      gobgpPod.Status.PodIP,
			as:           GoBgpAS,
			port:         GoBgpPort,
			name:         "test-peer",
			porterClient: porterClient,
			passive:      false, //test for passive
			forward:      true,  //test for port forward
		}
		bgppeer.Create()
		defer bgppeer.Delete()

		ginkgo.By("test bgpconf")
		bgpconf := &bgpTestGlobal{
			as:           PorterBgpAS,
			listenPort:   int32(PorterBgpPort),
			name:         "test-bgpconf",
			porterClient: porterClient,
			routerID:     "111111",
		}
		ginkgo.By("check bgpconf ip")
		//framework.ExpectError(bgpconf.Create())
		bgpconf.routerID = PorterRouterID
		framework.ExpectNoError(bgpconf.Create())
		defer bgpconf.Delete()

		ginkgo.By("add gobgp peer, port 179 for test port-forward")
		bgpClient.addPeerForGobgp(porterManagerPod.Status.PodIP, PorterBgpAS, 179)

		ginkgo.By("add service")
		tcpJig := e2eservice.NewTestJig(c, ns, "test-service")
		_, err = tcpJig.CreateTCPService(nil)
		framework.ExpectNoError(err)
		rc, err := tcpJig.Run(tcpJig.AddRCAntiAffinity)
		framework.ExpectNoError(err)
		//tcpJig.AddRCAntiAffinity(rc)
		_, err = tcpJig.UpdateService(func(s *v1.Service) {
			s.Spec.Type = v1.ServiceTypeLoadBalancer
			if s.ObjectMeta.Annotations == nil {
				s.ObjectMeta.Annotations = map[string]string{}
			}
			s.Annotations[porterconst.PorterAnnotationKey] = porterconst.PorterAnnotationValue
		})

		framework.ExpectNoError(err)
		tcpservice, err := tcpJig.WaitForLoadBalancer(defaultTime * time.Second)
		framework.ExpectNoError(err)
		framework.Logf("ingress %v", tcpservice.Status.LoadBalancer.Ingress)

		num := int(*rc.Spec.Replicas)
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, num))

		tcpJig.Scale(1)
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))

		ginkgo.By("add bgppeer twice should have no problem")
		bgppeer2 := &bgpTestPeer{
			address:      "9.9.9.9",
			as:           GoBgpAS + 1,
			port:         GoBgpPort + 1,
			name:         "test-peer2",
			porterClient: porterClient,
			passive:      false, //test for passive
			forward:      true,  //test for port forward
		}
		bgppeer2.Create()
		defer bgppeer2.Delete()
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))

		ginkgo.By("update bgp peer should have no prolem")
		bgppeer.port = GoBgpPort + 1
		bgppeer.Update()
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 0))
		ginkgo.By("restore bgp peer")
		bgppeer.port = GoBgpPort
		bgppeer.Update()
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))
		porterBgpClient.client.GetBgp(context.TODO(), &gobgpapi.GetBgpRequest{})

		ginkgo.By("update bgpconf should have no problem")
		bgpconf.Update(PorterBgpPort + 1)
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 0))
		ginkgo.By("restore bgpconf")
		bgpconf.Update(PorterBgpPort)
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))

		ginkgo.By("add bgpconf twice should have no problem")
		bgpconf2 := &bgpTestGlobal{
			as:           PorterBgpAS,
			listenPort:   int32(PorterBgpPort),
			name:         "test-bgpconf2",
			porterClient: porterClient,
			routerID:     PorterRouterID,
		}
		bgpconf2.Create()
		defer bgpconf2.Delete()
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))

		ginkgo.By("check eip status")
		porterClient.Get(context.TODO(), client.ObjectKey{Namespace: eip.Namespace, Name: eip.Name}, eip)
		framework.ExpectEqual(eip.Status, porterapi.EipStatus{
			Occupied: false,
			Usage:    1,
			PoolSize: 2,
		})

		tcpJig.Scale(0)
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 0))

		tcpJig.Scale(1)
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))

		//graceful shutdown
		ginkgo.By("graceful shutdown")
		c.CoreV1().Pods(PorterNamespace).Delete(context.TODO(), porterManagerPod.Name, metav1.DeleteOptions{})
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 1))
		framework.ExpectNoError(waitForRouterNum(defaultTime*time.Second, tcpservice.Status.LoadBalancer.Ingress[0].IP, bgpClient, 0))
	})
})

func waitForRouterNum(timeout time.Duration, ip string, bgpClient *gobgpClient, num int) error {
	pollFunc := func() (bool, error) {

		routers := bgpClient.getRoutersForGobgp(ip)
		if len(routers) != num {
			return false, nil
		} else {
			return true, nil
		}
	}
	if err := wait.PollImmediate(framework.Poll, timeout, pollFunc); err != nil {
		return err
	}
	return nil
}

func readFile(test, file string) string {
	from := filepath.Join(test, file)
	return commonutils.SubstituteImageName(string(testfiles.ReadOrDie(from)))
}
