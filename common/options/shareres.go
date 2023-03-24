package options

import (
	"bytes"
	"context"
	"fmt"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	log = ctrl.Log.WithName("options")
)

/*
return:
 1. resource interface{}
 2. canUpdate bool 资源是否允许存在时更新，如果为true，当资源已经存在时，会自动完成资源的增量更新
 3. err error
*/
type ResourcesCreator func() (res interface{}, canUpdate bool, err error)

/*
检查所需资源是否存在，如果不存在则创建，已存在则检查更新（根据是否允许更新标志来判断是否要增量更新）
*/
func SyncShareResources(client client.Client, reference metav1.OwnerReference, resFuncList ...ResourcesCreator) error {
	for _, resFunc := range resFuncList {
		resource, canUpdate, err := resFunc()
		if err != nil {
			return err
		}
		if resource == nil {
			continue
		}
		switch d := resource.(type) {
		case *v1.Service:
			service := &v1.Service{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: d.Name, Namespace: d.Namespace}, service)
			if err == nil && service.OwnerReferences == nil {
				// 如果service存在且OwnerReferences为空，则代表该集群为重建，且保留了原有的service配置
				service.OwnerReferences = []metav1.OwnerReference{reference}
				err = client.Update(context.TODO(), service)
				if err != nil {
					return fmt.Errorf("service update reference failed: %s. ", err.Error())
				}
				continue
			}
			if apierrors.IsNotFound(err) {
				d.OwnerReferences = []metav1.OwnerReference{reference}
				err = client.Create(context.TODO(), d)
				if err != nil {
					return err
				}
				continue
			} else if err != nil {
				return err
			}
			if !canUpdate {
				continue
			}
			// ports 增量变更
			newService := service.DeepCopy()
			portMaps := make(map[string]v1.ServicePort, 0)
			for _, port := range newService.Spec.Ports {
				portMaps[port.Name] = port
			}
			// 检查变更
			hasChange := false
			for _, port := range d.Spec.Ports {
				v, ok := portMaps[port.Name]
				if !ok {
					portMaps[port.Name] = port
					hasChange = true
					continue
				}
				port.NodePort = v.NodePort
				if !reflect.DeepEqual(v, port) {
					portMaps[port.Name] = port
					hasChange = true
					log.Info(fmt.Sprintf("pod has changed %#v, %#v. ", v, port))
				}
			}
			// 检查selector变更
			for !reflect.DeepEqual(newService.Spec.Selector, d.Spec.Selector) {
				newService.Spec.Selector = d.Spec.Selector
				hasChange = true
			}
			if hasChange {
				log.Info(fmt.Sprintf(" service %s has changed, do update. ", d.Name))
				newService.Spec.Ports = make([]v1.ServicePort, 0)
				for _, port := range portMaps {
					newService.Spec.Ports = append(newService.Spec.Ports, port)
				}
				err = client.Update(context.TODO(), newService)
				if err != nil {
					return fmt.Errorf("service update fail %s. ", err.Error())
				}
			}

		case *v1.ConfigMap:
			configMap := &v1.ConfigMap{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: d.Name, Namespace: d.Namespace}, configMap)
			if err == nil && configMap.OwnerReferences == nil {
				// 如果configmap存在且OwnerReferences为空，则代表该集群为重建，且保留了原有的configmap配置
				configMap.OwnerReferences = []metav1.OwnerReference{reference}
				err = client.Update(context.TODO(), configMap)
				if err != nil {
					return fmt.Errorf("configmap update reference failed: %s. ", err.Error())
				}
				continue
			}
			if apierrors.IsNotFound(err) {
				d.OwnerReferences = []metav1.OwnerReference{reference}
				err = client.Create(context.TODO(), d)
				if err != nil {
					return err
				}
				continue
			} else if err != nil {
				return err
			}
			if !canUpdate {
				continue
			}
			// data 增量变更
			hasChange := false
			newConfigMap := configMap.DeepCopy()
			if newConfigMap.Data == nil {
				if d.Data != nil {
					newConfigMap.Data = d.Data
					hasChange = true
				}
			} else {
				for k, v := range d.Data {
					if newConfigMap.Data[k] != v {
						newConfigMap.Data[k] = v
						hasChange = true
					}
				}
			}
			if hasChange {
				log.Info(fmt.Sprintf(" configMap %s data has changed, do update. ", d.Name))
				err = client.Update(context.TODO(), newConfigMap)
				if err != nil {
					return fmt.Errorf("configMap update fail %s. ", err.Error())
				}
			}
		case *v1.Secret:
			secret := &v1.Secret{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: d.Name, Namespace: d.Namespace}, secret)
			if err == nil && secret.OwnerReferences == nil {
				// 如果secret存在且OwnerReferences为空，则代表该集群为重建，且保留了原有的secret配置
				secret.OwnerReferences = []metav1.OwnerReference{reference}
				err = client.Update(context.TODO(), secret)
				if err != nil {
					return fmt.Errorf("secret update reference failed: %s. ", err.Error())
				}
				continue
			}
			if apierrors.IsNotFound(err) {
				d.OwnerReferences = []metav1.OwnerReference{reference}
				err = client.Create(context.TODO(), d)
				if err != nil {
					return err
				}
				continue
			} else if err != nil {
				return err
			}
			if !canUpdate {
				continue
			}
			// data 增量变更
			hasChange := false
			newSecret := secret.DeepCopy()
			if newSecret.Data == nil {
				if d.Data != nil {
					newSecret.Data = d.Data
					hasChange = true
				}
			} else {
				for k, v := range d.Data {
					if !bytes.Equal(newSecret.Data[k], v) {
						newSecret.Data[k] = v
						hasChange = true
					}
				}
			}
			if hasChange {
				log.Info(fmt.Sprintf("secret %s data has changed, do update. ", d.Name))
				err = client.Update(context.TODO(), newSecret)
				if err != nil {
					return fmt.Errorf("secret update fail %s. ", err.Error())
				}
			}
		default:
			log.Error(fmt.Errorf("unsupported type create :%v", d), "unsupported type")
			return fmt.Errorf("unsupported type create : %#v", d)
		}
	}

	return nil
}

// owner reference for resource both cluster and database
func ReferenceToOwner(reference metav1.OwnerReference, object metav1.Object) metav1.OwnerReference {
	ownerReference := reference.DeepCopy()
	ownerReference.UID = object.GetUID()
	ownerReference.Name = object.GetName()
	return *ownerReference
}
