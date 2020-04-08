package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strings"

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

func getKindHandler(kind string, kc *kubr.K8sResourceCache) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		podsJSON, _ := json.Marshal(kc.GetResourceList(kind))
		fmt.Fprintf(w, string(podsJSON)+"\n")
	}
}

func headers(w http.ResponseWriter, req *http.Request) {
	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
}

func main() {
	domain := flag.String("domain", "n.tripadvisor.com", "Domain name")
	cluster := flag.String("cluster", "ndmad2", "Kubernetes cluster")
	objsToCache := flag.String("cache", "namespaces,ingresses", "A comma-delimited list of Kubernetes object types to cache")
	flag.Parse()
	kinds := strings.Split(*objsToCache, ",")

	clientset, err := createClientSet()
	if err != nil {
		panic(err)
	}
	fmt.Println(fmt.Sprintf("Input params: Domain: %s, cluster: %s, kinds to cache: %s", *domain, *cluster, kinds))

	// kube resource cache (pointer)
	kc := kubr.NewK8sResourceCache(clientset, kinds)
	fmt.Println("NewK8sResource cache created")

	for _, kind := range kinds {
		http.HandleFunc("/"+kind, getKindHandler(kind, kc))
	}
	http.ListenAndServe(":8090", nil)
}
