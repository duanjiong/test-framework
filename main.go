package main

import (
	"fmt"
	"k8s.io/kubernetes/test/e2e/framework"
)

func main() {
	t := framework.NewDefaultFramework("haha")
	fmt.Printf(t.BaseName)
}
