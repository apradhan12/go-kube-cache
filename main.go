package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	//	"time"

	"k8s.io/client-go/kubernetes"

	kcfcf "github.com/phsiao/kcfcf"
	log "github.com/sirupsen/logrus"

	kubr "gitlab.dev.tripadvisor.com/PIT/go-kube-sidecar/kubresource"
)

var (
	k8scliflags *kcfcf.KCFCF
)

func init() {
	k8scliflags = kcfcf.NewKCFCF()
	k8scliflags.Init()
}

// create clientset to talk with k8s cluster
func createClientSet() (*kubernetes.Clientset, error) {
	config := k8scliflags.GetConfig()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return clientset, nil
}

func main() {
	domain := flag.String("domain", "n.tripadvisor.com", "Domain name")
	cluster := flag.String("cluster", "ndmad2", "Kubernetes cluster")

	flag.Parse()

	clientset, err := createClientSet()
	if err != nil {
		panic(err)
	}
	fmt.Println("Input params: Domain: ", *domain, "; cluster: ", *cluster)

	// kube resource cache (pointer)
	kinds := []string{"namespaces", "ingresses"}
	kc := kubr.NewK8sResourceCache(clientset, kinds)
	fmt.Println("NewK8sResource cache created ")

	ticker := time.NewTicker(time.Second * 2)
	go func() {
		for ; true; <-ticker.C {
			// TODO refine based on k8s store changes, add a channel to see if any changes are watched
			fmt.Println("we still here!")
			ingresses := kc.GetResourceList("ingresses")
			fmt.Println("we got the pods???!?!?!?!?!?")
			podsJSON, _ := json.Marshal(ingresses[0:10])
			fmt.Println("HERE IT IS: " + string(podsJSON))
		}
	}()
	for {
	}
}
