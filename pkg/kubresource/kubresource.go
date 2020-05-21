package kubresource

// talk to k8s API server to get the namespace objects and pods objects

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	networkingV1 "k8s.io/api/networking/v1"
	networkingV1Beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"

	// kubeinformers "k8s.io/client-go/informers"
	util_runtime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	// "k8s.io/client-go/tools/clientcmd/api"
	selector "github.com/apradhan12/go-kube-cache/pkg/selector"
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
		return &networkingV1Beta1.Ingress{}, clientset.NetworkingV1beta1().RESTClient()
	},
	"networkpolicies": func(clientset *kubernetes.Clientset) (runtime.Object, cache.Getter) {
		return &networkingV1.NetworkPolicy{}, clientset.NetworkingV1().RESTClient()
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
	log.Println("Refresh JSON outputs")
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
		log.Println(fmt.Sprintf("%s store synced", resourceKind))
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

// Does the given object have a field with the given value that can be obtained by following the path in the given slice?
func isFieldPresent(obj reflect.Value, fieldSlice []string, value string, isDeepestLevelMap bool) bool {
	log.Printf("Field slice: %s\n", fieldSlice)
	if len(fieldSlice) > 1 {
		isPresent, objVal := hasMatchingField(obj, fieldSlice[0])
		if isPresent {
			log.Printf("Checking field %s with type %s\n", fieldSlice[0], objVal.Type())
			return isFieldPresent(objVal, fieldSlice[1:], value, isDeepestLevelMap)
		}
	} else {
		if isDeepestLevelMap {
			resultMap, isMap := obj.Interface().(map[string]string)
			if isMap {
				actualValue, keyIsPresent := resultMap[fieldSlice[0]]
				if keyIsPresent {
					return actualValue == value
				}
			} else {
				log.Printf("The given object is not a map of strings: %s\n", obj.Interface())
			}
		} else {
			isPresent, objVal := hasMatchingField(obj, fieldSlice[0])
			if isPresent {
				log.Printf("Checking field %s with type %s\n", fieldSlice[0], objVal.Type())
				convertedStringValue := fmt.Sprintf("%s", objVal.Interface())
				log.Printf("Actual is '%s', looking for those that match '%s'\n", convertedStringValue, value)
				return convertedStringValue == value
			}
		}
	}
	return false
}

// LabelsField represents the path to labels given a map representing a k8s object
var LabelsField = []string{"metadata", "labels"}

// NamespaceField represents the path to namespace
var NamespaceField = []string{"metadata", "namespace"}

// matchesSelectors checks if the given reflected value matches all the given selectors
func matchesSelectors(obj reflect.Value, selectors []selector.Selector) (bool, error) {
	for _, selector := range selectors {
		for _, constraint := range strings.Split(selector.Contents, ",") {
			var matches bool
			switch kind := selector.Kind; kind {
			case "labelSelector":
				keyValuePair := strings.Split(constraint, "=")
				if len(keyValuePair) != 2 {
					return false, fmt.Errorf("Key and value are not matched correctly: %s", constraint)
				}
				matches = isFieldPresent(obj, append(LabelsField, keyValuePair[0]), keyValuePair[1], true)
			case "fieldSelector":
				keyValuePair := strings.Split(constraint, "=")
				if len(keyValuePair) != 2 {
					return false, fmt.Errorf("Key and value are not matched correctly: %s", constraint)
				}
				matches = isFieldPresent(obj, strings.Split(keyValuePair[0], "."), keyValuePair[1], false)
			case "namespace":
				matches = isFieldPresent(obj, NamespaceField, constraint, false)
			default:
				return false, fmt.Errorf("%s is not a valid selector kind", kind)
			}
			if !matches {
				return false, nil
			}
		}
	}
	return true, nil
}

// GetFilteredObjects gets the filtered objects of the given kind according to the given selector kind, key, and value.
func (p *K8sResourceCache) GetFilteredObjects(kind string, selectors []selector.Selector) ([]interface{}, error) {
	objects := p.GetResourceList(kind)
	filteredObjects := make([]interface{}, 0)
	i := 0
	for _, obj := range objects {
		log.Printf("Checking %d\n", i)
		matches, err := matchesSelectors(reflect.ValueOf(obj).Elem(), selectors)
		if err != nil {
			return nil, err
		}
		if matches {
			filteredObjects = append(filteredObjects, obj)
		}
		i++
	}
	return filteredObjects, nil
}

// hasMatchingField : Does the object have a JSON field matching the string? If so, returns the value of the field.
func hasMatchingField(obj reflect.Value, field string) (bool, reflect.Value) {
	// https://gist.github.com/drewolson/4771479
	val := obj
	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		tag := val.Type().Field(i).Tag
		jsonParams := strings.Split(tag.Get("json"), ",")
		if len(jsonParams) > 0 && jsonParams[0] == field {
			return true, valueField
		}
	}
	return false, reflect.Value{}
}
