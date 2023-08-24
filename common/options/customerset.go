/*


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

package options

import (
	"fmt"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strconv"
)

const (
	CustomerResCpu     = ".res.cpu"
	CustomerResMemory  = ".res.memory"
	CustomerResStorage = ".storage"

	ResourceRequest = ".request"
	ResourceLimit   = ".limit"
)

func GetResources(resourceUseType string, cpu, memory int64, cpuRota, memoryRota int64) *corev1.ResourceRequirements {

	res := corev1.ResourceRequirements{}
	res.Requests = make(corev1.ResourceList, 0)
	res.Limits = make(corev1.ResourceList, 0)

	if cpuRota <= 0 {
		cpuRota = 1
	}
	if memoryRota <= 0 {
		memoryRota = 1
	}

	switch resourceUseType {
	default:
		res.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu/cpuRota, resource.DecimalSI)
		res.Requests[corev1.ResourceMemory] = *resource.NewQuantity(memory/memoryRota, resource.BinarySI)

		res.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
		res.Limits[corev1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)
	case "owner":
		res.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
		res.Requests[corev1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)

		res.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
		res.Limits[corev1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)
	}
	return &res
}

func SetCustomerResources(setName string, resources *corev1.ResourceRequirements, customerSet map[string]string) {

	customerSet[setName+ResourceRequest+CustomerResCpu] = strconv.FormatInt(resources.Requests.Cpu().MilliValue(), 10)
	customerSet[setName+ResourceRequest+CustomerResMemory] = strconv.FormatInt(resources.Requests.Memory().Value(), 10)

	customerSet[setName+ResourceLimit+CustomerResCpu] = strconv.FormatInt(resources.Limits.Cpu().MilliValue(), 10)
	customerSet[setName+ResourceLimit+CustomerResMemory] = strconv.FormatInt(resources.Limits.Memory().Value(), 10)
}

func SetCustomerStorage(setName string, size int64, customerSet map[string]string) {
	customerSet[fmt.Sprintf("%s%s", setName, CustomerResStorage)] = strconv.FormatInt(size, 10)
}

func GetClusterTlsSecretName(clusterName string) string {
	return fmt.Sprintf("%s-tls", clusterName)
}

func GetClusterInnerTlsSecretName(clusterName string) string {
	return fmt.Sprintf("%s-inner-tls", clusterName)
}

func GetClusterMasterServiceName(clusterName string, isHeadless bool) string {
	svcName := fmt.Sprintf("%s-primary", clusterName)
	if isHeadless {
		return fmt.Sprintf("%s-headless", svcName)
	}
	return svcName
}

func GetClusterMasterServiceFullName(clusterName, namespace string, isHeadless bool) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", GetClusterMasterServiceName(clusterName, isHeadless), namespace)
}

func GetClusterSlaveServiceName(clusterName string, isHeadless bool) string {
	svcName := fmt.Sprintf("%s-slave", clusterName)
	if isHeadless {
		return fmt.Sprintf("%s-headless", svcName)
	}
	return svcName
}

func ClusterReadonlyServiceName(clusterName string, isHeadless bool) string {
	svcName := fmt.Sprintf("%s-readonly", clusterName)
	if isHeadless {
		return fmt.Sprintf("%s-headless", svcName)
	}
	return svcName
}

func GetClusterSlaveServiceFullName(clusterName, namespace string, isHeadless bool) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", GetClusterSlaveServiceName(clusterName, isHeadless), namespace)
}

func GetClusterSlaveDelayThresholdConfigName(clusterName string) string {
	return fmt.Sprintf("%s-slave-delay-threshold", clusterName)
}

func GetClusterReplicaSetName(clusterName string, index int) string {
	return fmt.Sprintf("%s-replica%d", clusterName, index)
}

func GetClusterHeadlessServiceName(clusterName string) string {
	return fmt.Sprintf("%s-headless", clusterName)
}

func GetHeadlessDomain(cluster *zookeeperv1.ZookeeperCluster) string {
	return fmt.Sprintf("%s.%s.svc.%s", cluster.Name, cluster.GetNamespace(), "cluster.local")
}

func GetClusterSlaveSetName(clusterName string, index int) string {
	return fmt.Sprintf("%s-slave%d", clusterName, index)
}

func GetSetServiceHeadlessName(setName string) string {
	return fmt.Sprintf("%s-headless", setName)
}

func GetServiceHeadlessNameById(clusterName string, id int) string {
	return fmt.Sprintf("%s-headless", GetClusterReplicaSetName(clusterName, id))
}

func GetServerDomain(setName, namespace string, id int, isCmd bool) string {
	if isCmd {
		return fmt.Sprintf("server.%d=%s:2888:3888:participant\\;2181", id, setName+"-headless."+namespace+".svc.cluster.local")
	}
	return fmt.Sprintf("server.%d=%s:2888:3888:participant;2181", id, setName+"-headless."+namespace+".svc.cluster.local")
}

func GetConnection(setName, namespace string, id int) string {
	return fmt.Sprintf("%s:2181", setName+"-headless."+namespace+".svc.cluster.local")
}

func GetClusterDBHeadless(namespace string, podName string) string {
	name := podName[:len(podName)-2]
	return fmt.Sprintf("%s-headless.%s.svc.cluster.local", name, namespace)
}

func GetClusterCustomizeConfigName(clusterName string) string {
	return fmt.Sprintf("%s-customize-config", clusterName)
}

func GetClusterLog4JQuietConfigName(clusterName string) string {
	return fmt.Sprintf("%s-log4j-quiet", clusterName)
}

func GetClusterLog4JConfigName(clusterName string) string {
	return fmt.Sprintf("%s-log4j", clusterName)
}
func GetClusterCustomConfigName(clusterName string) string {
	return fmt.Sprintf("%s-zoo-cfg", clusterName)
}

func GetClusterDynamicConfigName(clusterName string) string {
	return fmt.Sprintf("%s-zoo-dynamic", clusterName)
}
