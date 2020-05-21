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

	kubr "github.com/apradhan12/go-kube-cache/pkg/kubresource"
	selector "github.com/apradhan12/go-kube-cache/pkg/selector"
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

// ParseQueryParams converts a map of query params to a slice of selectors.
// The function assumes that there is exactly one query param, which is either
// fieldSelector or labelSelector.
func ParseQueryParams(queryParams map[string][]string) (selectors []selector.Selector) {
	selectors = make([]selector.Selector, 0)
	for selectorKind, valueList := range queryParams {
		// allows multiple selectors of the same kind
		for _, value := range valueList {
			if value != "" {
				selectors = append(selectors, selector.Selector{Kind: selectorKind, Contents: value})
			}
		}
	}
	return selectors
	// keyValuePair = strings.Split(queryParams[selectorKind][0], "=")
}

// PairLength represents the length of a slice representing a pair of key and value
var PairLength = 2

func getKindHandler(kind string, kc *kubr.K8sResourceCache) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		queryParamsMap := req.URL.Query()
		selectors := ParseQueryParams(queryParamsMap)
		if len(queryParamsMap) == 0 || len(selectors) == 0 {
			fmt.Fprintf(w, kc.GetJSONOutput(kind))
		} else {
			filteredObjects, err := kc.GetFilteredObjects(kind, selectors)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, err.Error())
			} else {
				jsonOutput, _ := json.Marshal(filteredObjects)
				fmt.Fprintf(w, string(jsonOutput))
			}
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
