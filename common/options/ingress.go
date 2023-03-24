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
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

const (
	IngressKey                 = "squids.io/ingress"
	NginxIngressName           = "nginx-ingress"
	HAProxyIngressName         = "haproxy-ingress"
	NodePortName               = "nodeport"
	HAProxyIngressConfigMapTCP = "haproxy-ingress-tcp"

	ProxyProtocolIn  = "proxy/proxyProtocolIn"  // 代理将解析接入流中的透传协议
	ProxyProtocolOut = "proxy/proxyProtocolOut" // 代理将透传协议发送到后端服务(需要后端服务解析)
)

func WithIngressAnnotation(annotation map[string]string, ingressType string) {
	annotation[IngressKey] = NodePortName
	if ingressType != "" {
		annotation[IngressKey] = ingressType
	}
}

/*
通知proxy向下游服务发送proxyprotocol
*/
func WithProxyProtocolOut(annotaion map[string]string) {
	annotaion[ProxyProtocolOut] = ""
}

func GetIngressType(annotation map[string]string) string {
	if annotation == nil {
		return ""
	}
	return annotation[IngressKey]
}

func GetIngressServiceExposedPort(svc *corev1.Service, portName string) (int32, error) {
	if svc.Annotations == nil {
		return 0, nil
	}

	var svcPort *corev1.ServicePort
	for _, port := range svc.Spec.Ports {
		if port.Name == portName {
			svcPort = &port
			break
		}
	}
	if svcPort == nil {
		return 0, fmt.Errorf("target port %s not found", portName)
	}

	var (
		exposedPort int
		err         error
	)

	switch GetIngressType(svc.Annotations) {
	case NginxIngressName:
		portStr, ok := svc.Annotations[fmt.Sprintf("%s/%d", NginxIngressName, svcPort.Port)]
		if !ok {
			return 0, fmt.Errorf("nginx ignress port not found in annotations")
		}
		exposedPort, err = strconv.Atoi(portStr)
		if err != nil {
			return 0, fmt.Errorf("faield to convert %s to int: %v", portStr, err)
		}
	case NodePortName:
		if svc.Spec.Type != corev1.ServiceTypeNodePort {
			return 0, fmt.Errorf("expected node port type, but found %s", svc.Spec.Type)
		}
		exposedPort = int(svcPort.NodePort)
	case HAProxyIngressName:
		portStr, ok := svc.Annotations[fmt.Sprintf("%s/%d", HAProxyIngressName, svcPort.Port)]
		if !ok {
			return 0, fmt.Errorf("haproxy ignress port not found in annotations")
		}
		exposedPort, err = strconv.Atoi(portStr)
		if err != nil {
			return 0, fmt.Errorf("faield to convert %s to int: %v", portStr, err)
		}
	}
	return int32(exposedPort), nil
}

// 判断一个服务暴露类型合法
func IngressTypeValid(t string) (valid bool) {
	switch t {
	case NginxIngressName, HAProxyIngressName, NodePortName:
		valid = true
	}
	return
}

// 判断为支持的ingress Controller
func IngressControllerProvided(t string) (provided bool) {
	switch t {
	case HAProxyIngressName, NginxIngressName:
		provided = true
	}
	return
}
