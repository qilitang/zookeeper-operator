package operator

import (
	"bytes"
	"fmt"
	"github.com/go-logr/logr"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	options "github.com/qilitang/zookeeper-operator/common/options"
	"github.com/qilitang/zookeeper-operator/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterSubResources struct {
	Cluster *zookeeperv1.ZookeeperCluster
	client.Client
	Log logr.Logger
}

func (t ClusterSubResources) createBaseService() (interface{}, bool, error) {
	svcSelectors := NewDatabaseLabel(t.Cluster)

	labels := NewDatabaseLabel(t.Cluster)
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

func (t ClusterSubResources) CreateService(clusterServiceType string) options.ResourcesCreator {
	return func() (res interface{}, canUpdate bool, err error) {
		baseService, canUpdate, err := t.createBaseService()
		if err != nil {
			return nil, canUpdate, err
		}
		service := baseService.(*corev1.Service)
		// Choose a service will be exposed
		if options.GetIngressType(t.Cluster.Annotations) != "" {
			clusterServiceType = options.GetIngressType(t.Cluster.Annotations)
		}
		service.Annotations = t.Cluster.DeepCopy().Annotations
		switch clusterServiceType {
		case options.NodePortName:
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
	service.Name = options.GetClusterHeadlessServiceName(t.Cluster.Name)
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
	service.Spec.ClusterIP = corev1.ClusterIPNone
	return service, true, nil
}

func (t ClusterSubResources) CreateLog4JQuietConfigMap() (interface{}, bool, error) {
	labels := NewDatabaseLabel(t.Cluster)
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.GetClusterLog4JQuietConfigName(t.Cluster.Name),
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
	labels := NewDatabaseLabel(t.Cluster)
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.GetClusterLog4JConfigName(t.Cluster.Name),
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
	labels := NewDatabaseLabel(t.Cluster)
	data := map[string]string{}
	utils.IncludeNonEmpty(data, "zoo.cfg", WithCustomConfig(t.Cluster))
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.GetClusterCustomConfigName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: data,
	}
	return &configmap, true, nil
}

func (t ClusterSubResources) CreateDynamicConfigMap() (interface{}, bool, error) {
	labels := NewDatabaseLabel(t.Cluster)
	data := map[string]string{}
	utils.IncludeNonEmpty(data, "zoo.cfg.dynamic", WithDynamicConfig(t.Cluster))
	configmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.GetClusterDynamicConfigName(t.Cluster.Name),
			Namespace: t.Cluster.Namespace,
			Labels:    labels,
		},
		Data: data,
	}
	return &configmap, true, nil
}

func (t ClusterSubResources) CreateReplicaHeadlessService(setName string) options.ResourcesCreator {
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
		service.Spec.ClusterIP = corev1.ClusterIPNone

		service.Name = setName + "-headless"
		labels := make(map[string]string, 0)
		labels[utils.SetName] = setName
		service.Spec.Selector = utils.CopyMap(labels)
		service.Labels = utils.CopyMap(labels)

		return service, canUpdate, nil
	}
}

func NewDatabaseLabel(cluster *zookeeperv1.ZookeeperCluster) map[string]string {
	labels := utils.CopyMap(cluster.Spec.Labels)
	labels[utils.AppNameLabelKey] = cluster.Name
	labels[utils.CreatedByLabelKey] = "qilitang"
	labels[utils.DBVersionLabelKey] = fmt.Sprintf("%s", cluster.Spec.Version)
	return labels
}

func WithCustomConfig(cluster *zookeeperv1.ZookeeperCluster) string {
	b := &bytes.Buffer{}
	utils.Iline(b, 0, "# custom zookeeper config")
	utils.Iline(b, 0, "4lw.commands.whitelist=*")
	utils.Iline(b, 0, "dataDir=/data")
	utils.Iline(b, 0, "standaloneEnabled=false")
	utils.Iline(b, 0, "reconfigEnabled=true")
	utils.Iline(b, 0, "skipACL=yes")
	utils.Iline(b, 0, "clientPort=2181")
	utils.Iline(b, 0, "metricsProvider.className=org.apache.zookeeper.metrics.prometheus.PrometheusMetricsProvider")
	//utils.Iline(b, 0, "metricsProvider.httpPort=7000")
	utils.Iline(b, 0, "metricsProvider.exportJvmInfo=true")
	utils.Iline(b, 0, "admin.serverPort=8080")
	utils.Iline(b, 0, "dynamicConfigFile=/conf/zoo.cfg.dynamic")
	cc := withDefaultConfig(cluster.Spec.ZookeeperCustomConf)
	utils.Iline(b, 0, fmt.Sprintf("initLimit=%d", cc.InitLimit))
	utils.Iline(b, 0, fmt.Sprintf("syncLimit=%d", cc.SyncLimit))
	utils.Iline(b, 0, fmt.Sprintf("tickTime=%d", cc.TickTime))
	utils.Iline(b, 0, fmt.Sprintf("globalOutstandingLimit=%d", cc.GlobalOutstandingLimit))
	utils.Iline(b, 0, fmt.Sprintf("preAllocSize=%d", cc.PreAllocSize))
	utils.Iline(b, 0, fmt.Sprintf("snapCount=%d", cc.SnapCount))
	utils.Iline(b, 0, fmt.Sprintf("commitLogCount=%d", cc.CommitLogCount))
	utils.Iline(b, 0, fmt.Sprintf("snapSizeLimitInKb=%d", cc.SnapSizeLimitInKb))
	utils.Iline(b, 0, fmt.Sprintf("maxCnxns=%d", cc.MaxCnxns))
	utils.Iline(b, 0, fmt.Sprintf("maxClientCnxns=%d", cc.MaxClientCnxns))
	utils.Iline(b, 0, fmt.Sprintf("minSessionTimeout=%d", cc.MinSessionTimeout))
	utils.Iline(b, 0, fmt.Sprintf("autopurge.snapRetainCount=%d", cc.AutoPurgeSnapRetainCount))
	utils.Iline(b, 0, fmt.Sprintf("autopurge.purgeInterval=%d", cc.AutoPurgePurgeInterval))
	utils.Iline(b, 0, fmt.Sprintf("quorumListenOnAllIPs=%s", "true"))
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

func WithDynamicConfig(cluster *zookeeperv1.ZookeeperCluster) string {
	b := &bytes.Buffer{}
	for i := 0; i < int(cluster.Spec.Replicas); i++ {
		setName := options.GetClusterReplicaSetName(cluster.Name, i)
		if i == int(cluster.Spec.Replicas)-1 {
			utils.Iline(b, 0, fmt.Sprintf("server.%d=%s:2888:3888:observer;2181", i, setName+"-headless."+cluster.Namespace+".svc.cluster.local"))
		} else {
			utils.Iline(b, 0, fmt.Sprintf("server.%d=%s:2888:3888:participant;2181", i, setName+"-headless."+cluster.Namespace+".svc.cluster.local"))
		}
	}
	return b.String()
}
func WithDynamicConfig1(cluster *zookeeperv1.ZookeeperCluster, index int) string {
	b := &bytes.Buffer{}
	for i := 0; i < index+1; i++ {
		setName := options.GetClusterReplicaSetName(cluster.Name, i)
		if i == int(cluster.Spec.Replicas)-1 {
			utils.Iline(b, 0, fmt.Sprintf("server.%d=%s:2888:3888:observer;0.0.0.0:2181", i, setName+"-headless."+cluster.Namespace+".svc.cluster.local"))
		} else {
			utils.Iline(b, 0, fmt.Sprintf("server.%d=%s:2888:3888:participant;0.0.0.0:2181", i, setName+"-headless."+cluster.Namespace+".svc.cluster.local"))
		}
	}
	return b.String()
}
