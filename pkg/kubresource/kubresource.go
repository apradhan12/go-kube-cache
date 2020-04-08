package kubresource

// talk to k8s API server to get the namespace objects and pods objects

import (
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
}

// NewK8sResourceCache ... get k8s resources in the cluster
func NewK8sResourceCache(clientset *kubernetes.Clientset, kinds []string) *K8sResourceCache {
	if clientset == nil {
		panic(errors.New("clientset is nil"))
	}

	kc := K8sResourceCache{
		stores: make(map[string]cache.Store),
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
	return &kc
}

// GetResourceList ... Gets resource list
func (cache *K8sResourceCache) GetResourceList(kind string) []interface{} {
	return cache.stores[kind].List()
}
