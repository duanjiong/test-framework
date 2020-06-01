package network

import "github.com/onsi/ginkgo"

// KubesphereDescribe annotates the test with the SIG label.
func KubesphereDescribe(text string, body func()) bool {
	return ginkgo.Describe("[sig-network] "+text, body)
}
