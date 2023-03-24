/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ZookeeperClusterSpec defines the desired state of ZookeeperCluster
type ZookeeperClusterSpec struct {

	// Replicas is the cluster node counts
	Replicas int32 `json:"replicas"`

	// Labels specifies the labels to attach to statefulsets the operator creates for
	// the zookeeper cluster.
	Labels map[string]string `json:"labels,omitempty"`

	// Version is the zookeeper version
	Version string `json:"version,omitempty"`
	// Image is the  container image. default is zookeeper:3.5
	Image string `json:"image"`
	//
	ZookeeperCustomConf ZookeeperConfig `json:"config,omitempty"`

	// ZookeeperResources describes the cluster database compute resource requirements
	ZookeeperResources *ZookeeperResources `json:"zookeeperResources,omitempty"`
}

// ZookeeperConfig is the current configuration of each Zookeeper node, which
// sets these values in the config-map
type ZookeeperConfig struct {
	// InitLimit is the amount of time, in ticks, to allow followers to connect
	// and sync to a leader.
	//
	// Default value is 10.
	InitLimit int `json:"initLimit,omitempty"`

	// TickTime is the length of a single tick, which is the basic time unit used
	// by Zookeeper, as measured in milliseconds
	//
	// The default value is 2000.
	TickTime int `json:"tickTime,omitempty"`

	// SyncLimit is the amount of time, in ticks, to allow followers to sync with
	// Zookeeper.
	//
	// The default value is 2.
	SyncLimit int `json:"syncLimit,omitempty"`

	// Clients can submit requests faster than ZooKeeper can process them, especially
	// if there are a lot of clients. Zookeeper will throttle Clients so that requests
	// won't exceed global outstanding limit.
	//
	// The default value is 1000
	GlobalOutstandingLimit int `json:"globalOutstandingLimit,omitempty"`

	// To avoid seeks ZooKeeper allocates space in the transaction log file in
	// blocks of preAllocSize kilobytes
	//
	// The default value is 64M
	PreAllocSize int `json:"preAllocSize,omitempty"`

	// ZooKeeper records its transactions using snapshots and a transaction log
	// The number of transactions recorded in the transaction log before a snapshot
	// can be taken is determined by snapCount
	//
	// The default value is 100,000
	SnapCount int `json:"snapCount,omitempty"`

	// Zookeeper maintains an in-memory list of last committed requests for fast
	// synchronization with followers
	//
	// The default value is 500
	CommitLogCount int `json:"commitLogCount,omitempty"`

	// Snapshot size limit in Kb
	//
	// The defult value is 4GB
	SnapSizeLimitInKb int `json:"snapSizeLimitInKb,omitempty"`

	// Limits the total number of concurrent connections that can be made to a
	//zookeeper server
	//
	// The defult value is 0, indicating no limit
	MaxCnxns int `json:"maxCnxns,omitempty"`

	// Limits the number of concurrent connections that a single client, identified
	// by IP address, may make to a single member of the ZooKeeper ensemble.
	//
	// The default value is 60
	MaxClientCnxns int `json:"maxClientCnxns,omitempty"`

	// The minimum session timeout in milliseconds that the server will allow the
	// client to negotiate
	//
	// The default value is 4000
	MinSessionTimeout int `json:"minSessionTimeout,omitempty"`

	// The maximum session timeout in milliseconds that the server will allow the
	// client to negotiate.
	//
	// The default value is 40000
	MaxSessionTimeout int `json:"maxSessionTimeout,omitempty"`

	// Retain the snapshots according to retain count
	//
	// The default value is 3
	AutoPurgeSnapRetainCount int `json:"autoPurgeSnapRetainCount,omitempty"`

	// The time interval in hours for which the purge task has to be triggered
	//
	// Disabled by default
	AutoPurgePurgeInterval int `json:"autoPurgePurgeInterval,omitempty"`

	// QuorumListenOnAllIPs when set to true the ZooKeeper server will listen for
	// connections from its peers on all available IP addresses, and not only the
	// address configured in the server list of the configuration file. It affects
	// the connections handling the ZAB protocol and the Fast Leader Election protocol.
	//
	// The default value is false.
	QuorumListenOnAllIPs bool `json:"quorumListenOnAllIPs,omitempty"`

	// key-value map of additional zookeeper configuration parameters
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	AdditionalConfig map[string]string `json:"additionalConfig,omitempty"`
}

// +kubebuilder:object:generate:root=true
type ZookeeperResources struct {

	// Resources describes the compute resource requirements(include cpu、memory)
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage describes database persistent volume related attributes
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`
}

type StorageSpec struct {
	// Size describes database persistent volume storage
	Size *int64 `json:"size"`

	// StorageClass describes database volume storageClass definition
	// If not specified, use default StorageClass
	// +optional
	StorageClass *string `json:"storageClass,omitempty"`
}

// ZookeeperClusterStatus defines the observed state of ZookeeperCluster
type ZookeeperClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ClusterStatus `json:",inline"`
}

// +kubebuilder:object:generate:root=true
type ClusterStatus struct {
	// observedGeneration is the most recent generation observed for this StatefulSet. It corresponds to the
	// StatefulSet's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// replicas is the number of Pods created by the StatefulSet controller.
	Replicas int32 `json:"replicas" protobuf:"varint,2,opt,name=replicas"`

	// readyReplicas is the number of Pods created by the StatefulSet controller that have a Ready Condition.
	ReadyReplicas int32 `json:"readyReplicas,omitempty" protobuf:"varint,3,opt,name=readyReplicas"`

	// CurrentReplicas is the number of Pods created by the StatefulSet controller from the StatefulSet version
	// indicated by currentRevision.
	CurrentReplicas int32 `json:"currentReplicas,omitempty" protobuf:"varint,4,opt,name=currentReplicas"`

	// updatedReplicas is the number of Pods created by the StatefulSet controller from the StatefulSet version
	// indicated by updateRevision.
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty" protobuf:"varint,5,opt,name=updatedReplicas"`

	// currentRevision, if not empty, indicates the version of the StatefulSet used to generate Pods in the
	// sequence [0,currentReplicas).
	CurrentRevision string `json:"currentRevision,omitempty" protobuf:"bytes,6,opt,name=currentRevision"`

	// updateRevision, if not empty, indicates the version of the StatefulSet used to generate Pods in the sequence
	// [replicas-updatedReplicas,replicas)
	UpdateRevision string `json:"updateRevision,omitempty" protobuf:"bytes,7,opt,name=updateRevision"`

	// collisionCount is the count of hash collisions for the StatefulSet. The StatefulSet controller
	// uses this field as a collision avoidance mechanism when it needs to create the name for the
	// newest ControllerRevision.
	// +optional
	CollisionCount *int32 `json:"collisionCount,omitempty" protobuf:"varint,9,opt,name=collisionCount"`

	CustomStatus  string `json:"customStatus,omitempty"`
	FSMStatus     string `json:"fsmStatus,omitempty"`
	StatusDetails string `json:"statusDetails,omitempty"`

	PermitRollback bool `json:"PermitRollback"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=zzk
// ZookeeperCluster is the Schema for the zookeeperclusters API
type ZookeeperCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ZookeeperClusterSpec   `json:"spec,omitempty"`
	Status ZookeeperClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ZookeeperClusterList contains a list of ZookeeperCluster
type ZookeeperClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZookeeperCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ZookeeperCluster{}, &ZookeeperClusterList{})
}
