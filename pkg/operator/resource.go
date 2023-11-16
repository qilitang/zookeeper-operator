package operator

import (
	"bytes"
	"context"
	"fmt"
	"github.com/go-logr/logr"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	options2 "github.com/qilitang/zookeeper-operator/pkg/common/options"
	utils2 "github.com/qilitang/zookeeper-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type ClusterSubResources struct {
	Cluster *zookeeperv1.ZookeeperCluster
	client.Client
	Log logr.Logger
}

func (t ClusterSubResources) createBaseService() (interface{}, bool, error) {
	svcSelectors := NewClusterLabel(t.Cluster)

	labels := NewClusterLabel(t.Cluster)
	service := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.Cluster.Name,
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector:        svcSelectors,
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp-client",
					Protocol:   corev1.ProtocolTCP,
					Port:       2181,
					TargetPort: intstr.FromInt(2181),
				},
				{
					Name:       "tcp-quorum",
					Protocol:   corev1.ProtocolTCP,
					Port:       2888,
					TargetPort: intstr.FromInt(2888),
				},
				{
					Name:       "tcp-leader-election",
					Protocol:   corev1.ProtocolTCP,
					Port:       3888,
					TargetPort: intstr.FromInt(3888),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return &service, true, nil
}

func (t ClusterSubResources) CreateService(clusterServiceType string) options2.ResourcesCreator {
	return func() (res interface{}, canUpdate bool, err error) {
		baseService, canUpdate, err := t.createBaseService()
		if err != nil {
			return nil, canUpdate, err
		}
		service := baseService.(*corev1.Service)
		service.Annotations = t.Cluster.DeepCopy().Annotations
		switch clusterServiceType {
		case options2.NodePortName:
			service.Spec.Type = corev1.ServiceTypeNodePort
		}
		return service, true, nil
	}
}

func (t ClusterSubResources) CreateHeadlessService() (interface{}, bool, error) {
	baseService, canUpdate, err := t.createBaseService()
	if err != nil {
		return nil, canUpdate, err
	}
	service := baseService.(*corev1.Service)
	service.Name = options2.GetClusterHeadlessServiceName(t.Cluster.Name)
	if service.Annotations == nil {
		service.Annotations = make(map[string]string, 0)
	}
	// for createBackupContainer ssh command
	service.Spec.Ports = append(service.Spec.Ports, []corev1.ServicePort{
		{
			Name:       "tcp-metrics",
			Protocol:   corev1.ProtocolTCP,
			Port:       7000,
			TargetPort: intstr.FromInt(7000),
		},
		{
			Name:       "tcp-admin-server",
			Protocol:   corev1.ProtocolTCP,
			Port:       8080,
			TargetPort: intstr.FromInt(8080),
		},
	}...)
	// headless
	service.Spec.Type = corev1.ServiceTypeClusterIP
	//service.Spec.ClusterIP = corev1.ClusterIPNone
	return service, true, nil
}

func (t ClusterSubResources) CreateLog4JQuietConfigMap() (interface{}, bool, error) {
	labels := NewClusterLabel(t.Cluster)
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options2.GetClusterLog4JQuietConfigName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"log4j-quiet.properties": ZKLog4JQuietConf,
		},
	}
	return &configmap, true, nil
}

func (t ClusterSubResources) CreateLog4JConfigMap() (interface{}, bool, error) {
	labels := NewClusterLabel(t.Cluster)
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options2.GetClusterLog4JConfigName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"log4j.properties": ZkLog4JConf,
		},
	}
	return &configmap, true, nil
}

func (t ClusterSubResources) CreateCustomConfigMap() (interface{}, bool, error) {
	labels := NewClusterLabel(t.Cluster)
	data := map[string]string{}
	utils2.IncludeNonEmpty(data, "zoo.cfg", WithCustomConfig(t.Cluster))
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options2.GetClusterCustomConfigName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: data,
	}
	return &configmap, true, nil
}
func (t ClusterSubResources) CreateScriptConfigMap() (interface{}, bool, error) {
	labels := NewClusterLabel(t.Cluster)
	data := map[string]string{}
	utils2.IncludeNonEmpty(data, "script.sh", `
#!/bin/bash

output=$(echo stat | nc localhost 2181 2>/dev/null)

if [[ $output == *"Mode: leader"* ]]; then
    echo "Zookeeper is in Mode: leader"
    exit 0
elif [[ $output == *"Mode: follower"* ]]; then
    echo "Zookeeper is in Mode: follower"
    exit 0
else
    echo "Zookeeper mode is unknown or not detected."
    exit 1
fi
`)
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options2.CreateScriptConfigMapName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: data,
	}
	return &configmap, true, nil
}

func (t ClusterSubResources) CreateDynamicConfigMap(ctx context.Context, serverIndex int) options2.ResourcesCreator {
	return func() (res interface{}, canUpdate bool, err error) {
		key := "zoo.cfg.dynamic"
		info := WithDynamicConfig(t.Cluster, serverIndex)
		cm := &corev1.ConfigMap{}
		err = t.Get(ctx, types.NamespacedName{Name: options2.GetClusterDynamicConfigName(t.Cluster.Name), Namespace: t.Cluster.Namespace}, cm)
		if err != nil && errors.IsNotFound(err) {
			return t.createDynamicConfig(key, info), true, nil
		}
		if err != nil {
			return cm, false, fmt.Errorf("get dynamic configmap failed: %s", err)
		}
		if strings.Contains(cm.Data[key], info) {
			return cm, false, nil
		}
		return t.createDynamicConfig(key, info), true, nil
	}
}

func (t ClusterSubResources) createDynamicConfig(key, info string) *corev1.ConfigMap {
	labels := NewClusterLabel(t.Cluster)
	data := map[string]string{}
	utils2.IncludeNonEmpty(data, key, info)
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options2.GetClusterDynamicConfigName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: data,
	}
	return configmap
}

func (t ClusterSubResources) CreateReplicaHeadlessService(setName string) options2.ResourcesCreator {
	return func() (res interface{}, canUpdate bool, err error) {
		baseService, canUpdate, err := t.createBaseService()
		if err != nil {
			return nil, canUpdate, err
		}
		service := baseService.(*corev1.Service)

		if service.Annotations == nil {
			service.Annotations = make(map[string]string, 0)
		}
		service.Annotations["service.alpha.kubernetes.io/tolerate-unready-endpoints"] = "true"

		// headless
		service.Spec.Type = corev1.ServiceTypeClusterIP
		//service.Spec.ClusterIP = corev1.ClusterIPNone

		service.Name = options2.GetSetServiceHeadlessName(setName)
		labels := make(map[string]string, 0)
		labels[utils2.SetName] = setName
		service.Spec.Selector = utils2.CopyMap(labels)
		service.Labels = utils2.CopyMap(labels)

		return service, canUpdate, nil
	}
}

func NewClusterLabel(cluster *zookeeperv1.ZookeeperCluster) map[string]string {
	labels := utils2.CopyMap(cluster.Spec.Labels)
	labels[utils2.AppNameLabelKey] = cluster.Name
	labels[utils2.CreatedByLabelKey] = "qilitang"
	return labels
}
func NewClusterAnnotations(cluster *zookeeperv1.ZookeeperCluster) map[string]string {
	annotations := utils2.CopyMap(cluster.Annotations)
	annotations[utils2.AnnotationsRoleKey] = utils2.AnnotationsRoleNotReady
	return annotations
}

func WithCustomConfig(cluster *zookeeperv1.ZookeeperCluster) string {
	b := &bytes.Buffer{}
	utils2.Iline(b, 0, "# custom zookeeper config")
	utils2.Iline(b, 0, "4lw.commands.whitelist=*")
	utils2.Iline(b, 0, "dataDir=/data")
	utils2.Iline(b, 0, "standaloneEnabled=false")
	utils2.Iline(b, 0, "reconfigEnabled=true")
	utils2.Iline(b, 0, "skipACL=yes")
	utils2.Iline(b, 0, "clientPort=2181")
	utils2.Iline(b, 0, "metricsProvider.className=org.apache.zookeeper.metrics.prometheus.PrometheusMetricsProvider")
	//utils.Iline(b, 0, "metricsProvider.httpPort=7000")
	utils2.Iline(b, 0, "metricsProvider.exportJvmInfo=true")
	utils2.Iline(b, 0, "admin.serverPort=8080")
	utils2.Iline(b, 0, "dynamicConfigFile=/conf/zoo.cfg.dynamic")
	cc := withDefaultConfig(cluster.Spec.ZookeeperCustomConf)
	utils2.Iline(b, 0, fmt.Sprintf("initLimit=%d", cc.InitLimit))
	utils2.Iline(b, 0, fmt.Sprintf("syncLimit=%d", cc.SyncLimit))
	utils2.Iline(b, 0, fmt.Sprintf("tickTime=%d", cc.TickTime))
	utils2.Iline(b, 0, fmt.Sprintf("globalOutstandingLimit=%d", cc.GlobalOutstandingLimit))
	utils2.Iline(b, 0, fmt.Sprintf("preAllocSize=%d", cc.PreAllocSize))
	utils2.Iline(b, 0, fmt.Sprintf("snapCount=%d", cc.SnapCount))
	utils2.Iline(b, 0, fmt.Sprintf("commitLogCount=%d", cc.CommitLogCount))
	utils2.Iline(b, 0, fmt.Sprintf("snapSizeLimitInKb=%d", cc.SnapSizeLimitInKb))
	utils2.Iline(b, 0, fmt.Sprintf("maxCnxns=%d", cc.MaxCnxns))
	utils2.Iline(b, 0, fmt.Sprintf("maxClientCnxns=%d", cc.MaxClientCnxns))
	utils2.Iline(b, 0, fmt.Sprintf("minSessionTimeout=%d", cc.MinSessionTimeout))
	utils2.Iline(b, 0, fmt.Sprintf("autopurge.snapRetainCount=%d", cc.AutoPurgeSnapRetainCount))
	utils2.Iline(b, 0, fmt.Sprintf("autopurge.purgeInterval=%d", cc.AutoPurgePurgeInterval))
	utils2.Iline(b, 0, fmt.Sprintf("quorumListenOnAllIPs=%s", "true"))
	return b.String()
}

func withDefaultConfig(zc zookeeperv1.ZookeeperConfig) zookeeperv1.ZookeeperConfig {
	defaultConfig := zookeeperv1.ZookeeperConfig{
		InitLimit:                5,
		TickTime:                 2000,
		SyncLimit:                10,
		GlobalOutstandingLimit:   1000,
		PreAllocSize:             65536,
		SnapCount:                10000,
		CommitLogCount:           500,
		SnapSizeLimitInKb:        4194304,
		MaxClientCnxns:           60,
		AutoPurgeSnapRetainCount: 3,
	}

	if zc.InitLimit == 0 {
		zc.InitLimit = defaultConfig.InitLimit
	}
	if zc.TickTime == 0 {
		zc.TickTime = defaultConfig.TickTime
	}
	if zc.SyncLimit == 0 {
		zc.SyncLimit = defaultConfig.SyncLimit
	}
	if zc.GlobalOutstandingLimit == 0 {
		zc.GlobalOutstandingLimit = defaultConfig.GlobalOutstandingLimit
	}
	if zc.PreAllocSize == 0 {
		zc.PreAllocSize = defaultConfig.PreAllocSize
	}
	if zc.SnapCount == 0 {
		zc.SnapCount = defaultConfig.SnapCount
	}
	if zc.CommitLogCount == 0 {
		zc.CommitLogCount = defaultConfig.CommitLogCount
	}
	if zc.SnapSizeLimitInKb == 0 {
		zc.SnapSizeLimitInKb = defaultConfig.SnapSizeLimitInKb
	}
	if zc.MaxClientCnxns == 0 {
		zc.MaxClientCnxns = defaultConfig.MaxClientCnxns
	}
	if zc.MinSessionTimeout == 0 {
		zc.MinSessionTimeout = 2 * zc.TickTime
	}
	if zc.MaxSessionTimeout == 0 {
		zc.MaxSessionTimeout = 20 * zc.TickTime
	}
	if zc.AutoPurgeSnapRetainCount == 0 {
		zc.AutoPurgeSnapRetainCount = defaultConfig.AutoPurgeSnapRetainCount
	}

	return zc
}

func WithDynamicConfig(cluster *zookeeperv1.ZookeeperCluster, serverIndex int) string {
	b := &bytes.Buffer{}
	for i := 0; i < serverIndex+1; i++ {
		setName := options2.GetClusterReplicaSetName(cluster.Name, i)
		utils2.Iline(b, 0, options2.GetServerDomain(setName, cluster.Namespace, i, false))
	}
	return b.String()
}
