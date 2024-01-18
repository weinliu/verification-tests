package hypershift

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

type watchInfo struct {
	// svc to watch
	namespace    string
	resourceType K8SResource

	// event handler func
	addFunc    func(obj interface{})
	updateFunc func(oldObj interface{}, newObj interface{})
	deleteFunc func(obj interface{})

	//name of resource, informer only watches the crd in the namespace instead of resource name
	//you need to compare the resource name in the event handler to check if it is the target name
	name string
}

func startWatch(ctx context.Context, kubeconfigPath string, info watchInfo) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	go staticWatch(ctx, clientSet, info)
	return nil
}

func staticWatch(ctx context.Context, clientSet kubernetes.Interface, info watchInfo) {
	fac := informers.NewSharedInformerFactoryWithOptions(clientSet, 0, informers.WithNamespace(info.namespace))

	var informer cache.SharedIndexInformer
	switch info.resourceType {
	case Service:
		informer = fac.Core().V1().Services().Informer()
	default:
		e2e.Logf("invalid resource type %s, return", string(info.resourceType))
		return
	}

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    info.addFunc,
		DeleteFunc: info.deleteFunc,
		UpdateFunc: info.updateFunc,
	})
	if err != nil {
		e2e.Logf("AddEventHandler err for resource %s %s in %s, err %s, return", string(info.resourceType), info.name, info.namespace, err.Error())
		return
	}

	e2e.Logf("start informer event watch for %s: %s %s", string(info.resourceType), info.namespace, info.name)
	informer.Run(ctx.Done())
	e2e.Logf("ctx Done %s, exit watching %s: %s %s", ctx.Err(), string(info.resourceType), info.namespace, info.name)
}

type operatorWatchInfo struct {
	//CRD GVR to watch
	group     string
	version   string
	resources string

	// CR namespace to watch
	namespace string

	// event handler func, Parameter []byte can be used to Unmarshal into the specified crd structure
	addFunc    func(obj []byte)
	updateFunc func(oldObj []byte, newObj []byte)
	deleteFunc func(obj []byte)

	//name of cr resource, informer only watches the crd in the namespace instead of resource name
	//you need to compare the resource name in the event handler to check if it is the target name
	name string
}

func startWatchOperator(ctx context.Context, kubeconfigPath string, info operatorWatchInfo) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return err
	}
	go watchOperator(ctx, dynamicClient, info)
	return nil
}

func watchOperator(ctx context.Context, client dynamic.Interface, info operatorWatchInfo) {
	fac := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, 0, info.namespace, nil)
	informer := fac.ForResource(schema.GroupVersionResource{
		Group:    info.group,
		Version:  info.version,
		Resource: info.resources,
	}).Informer()

	eventHandler := cache.ResourceEventHandlerFuncs{}
	if info.addFunc != nil {
		eventHandler.AddFunc = func(obj interface{}) {
			typedObj := obj.(*unstructured.Unstructured)
			bytes, _ := typedObj.MarshalJSON()
			info.addFunc(bytes)
		}
	}

	if info.deleteFunc != nil {
		eventHandler.DeleteFunc = func(obj interface{}) {
			typedObj := obj.(*unstructured.Unstructured)
			bytes, _ := typedObj.MarshalJSON()
			info.deleteFunc(bytes)
		}
	}

	if info.updateFunc != nil {
		eventHandler.UpdateFunc = func(oldObj interface{}, newObj interface{}) {
			typedObj := oldObj.(*unstructured.Unstructured)
			oldObjBytes, err := typedObj.MarshalJSON()
			if err != nil {
				return
			}

			typedObj = newObj.(*unstructured.Unstructured)
			newObjBytes, err := typedObj.MarshalJSON()
			if err != nil {
				return
			}

			info.updateFunc(oldObjBytes, newObjBytes)
		}
	}

	_, err := informer.AddEventHandler(eventHandler)
	if err != nil {
		e2e.Logf("AddEventHandler err for %s %s in %s, err %s, return", info.resources, info.name, info.namespace, err.Error())
		return
	}

	e2e.Logf("start informer event watch for %s.%s %s %s", info.resources, info.group, info.namespace, info.name)
	informer.Run(ctx.Done())
	e2e.Logf("ctx Done %s, exit watching %s.%s %s %s", ctx.Err(), info.resources, info.group, info.namespace, info.name)
}
