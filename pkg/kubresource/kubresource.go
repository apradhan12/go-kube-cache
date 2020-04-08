package kubresource

// talk to k8s API server to get the namespace objects and pods objects

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"

	// kubeinformers "k8s.io/client-go/informers"
	util_runtime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	// "k8s.io/client-go/tools/clientcmd/api"
)

// K8sResourceCache ... local cache for faster API serving
type K8sResourceCache struct {
	stores map[string]cache.Store
	output map[string]string
	mux    sync.Mutex
}

var typeMap = map[string]func(clientset *kubernetes.Clientset) (runtime.Object, cache.Getter){
	"namespaces": func(clientset *kubernetes.Clientset) (runtime.Object, cache.Getter) {
		return &v1.Namespace{}, clientset.CoreV1().RESTClient()
	},
	"pods": func(clientset *kubernetes.Clientset) (runtime.Object, cache.Getter) {
		return &v1.Pod{}, clientset.CoreV1().RESTClient()
	},
	"ingresses": func(clientset *kubernetes.Clientset) (runtime.Object, cache.Getter) {
		return &v1beta1.Ingress{}, clientset.NetworkingV1beta1().RESTClient()
	},
	"networkpolicies": func(clientset *kubernetes.Clientset) (runtime.Object, cache.Getter) {
		return &v1beta1.NetworkPolicy{}, clientset.NetworkingV1beta1().RESTClient()
	},
}

func (p *K8sResourceCache) updateJSONOutput() {
	p.mux.Lock()
	stores := p.stores
	p.mux.Unlock()

	for kind, store := range stores {
		jsonOutput, _ := json.Marshal(store.List())
		p.mux.Lock()
		p.output[kind] = string(jsonOutput)
		p.mux.Unlock()
	}
}

func (p *K8sResourceCache) updateK8sResourceCache() {
	fmt.Println("Refresh JSON outputs")
	p.updateJSONOutput()
}

// NewK8sResourceCache ... get k8s resources in the cluster
func NewK8sResourceCache(clientset *kubernetes.Clientset, kinds []string) *K8sResourceCache {
	if clientset == nil {
		panic(errors.New("clientset is nil"))
	}

	kc := K8sResourceCache{
		stores: make(map[string]cache.Store),
		output: make(map[string]string),
	}

	for _, resourceKind := range kinds {
		runtimeObj, cacheGetter := typeMap[resourceKind](clientset)
		watchList := cache.NewListWatchFromClient(cacheGetter, resourceKind, v1.NamespaceAll, fields.Everything())
		store, controller := cache.NewInformer(
			watchList,
			runtimeObj,
			time.Second*30,
			cache.ResourceEventHandlerFuncs{},
		)
		go controller.Run(wait.NeverStop)
		if !cache.WaitForCacheSync(wait.NeverStop, controller.HasSynced) {
			util_runtime.HandleError(fmt.Errorf("Timed out waiting for %s caches to sync", resourceKind))
		}
		fmt.Println(fmt.Sprintf("%s store synced", resourceKind))
		kc.stores[resourceKind] = store
	}

	ticker := time.NewTicker(time.Second * 30)
	go func() {
		for ; true; <-ticker.C {
			// TODO refine based on k8s store changes, add a channel to see if any changes are watched
			kc.updateK8sResourceCache()
		}
	}()

	return &kc
}

// GetResourceList ... Gets resource list
func (p *K8sResourceCache) GetResourceList(kind string) []interface{} {
	p.mux.Lock()
	resources := p.stores[kind].List()
	p.mux.Unlock()
	return resources
}

// GetJSONOutput ... Gets JSON output
func (p *K8sResourceCache) GetJSONOutput(kind string) string {
	p.mux.Lock()
	jsonOutput := p.output[kind]
	p.mux.Unlock()
	return jsonOutput
}
