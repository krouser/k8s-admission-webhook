package main

import (
	"fmt"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
	"strings"
)

var ingressHostRegExp = regexp.MustCompile(`^(([a-zA-Z])|([a-zA-Z][a-zA-Z])|([a-zA-Z][0-9])|([0-9][a-zA-Z])|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3})$`)
var ingressPathRegExp = regexp.MustCompile(`^[A-zA-Z0-9_/.\-]*$`)

type TlsDefinition struct {
	host             string
	secretName       string
	ingressName      string
	ingressNamespace string
}

type PathDefinition struct {
	host        string
	path        string
	serviceName string
	servicePort string
}

func (pathDefinition *PathDefinition) toUri() string {
	return pathDefinition.host + pathDefinition.path
}

func validateIngress(validation *objectValidation, ingress *extv1beta1.Ingress, config *config) error {

	targetDesc := fmt.Sprintf("Ingress %s", ingress.Name)

	remoteIngresses, err := ingressClient().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	logger.Debugf("There are %d ingresses in the cluster\n", len(remoteIngresses.Items))

	localPathData := ingressPath(ingress)
	localTlsData := ingressTls(ingress)

	if config.RuleIngressRegex {
		validatePathDataRegex(localPathData, validation, targetDesc)
		validateTlsDataRegex(localTlsData, validation, targetDesc)
	}

	if config.RuleIngressCollision {
		var remotePathData []PathDefinition
		for _, remoteIngress := range remoteIngresses.Items {
			remotePathData = append(remotePathData, ingressPath(&remoteIngress)...)
		}
		validatePathDataCollision(localPathData, remotePathData, validation, targetDesc)

		var remoteTlsData []TlsDefinition
		for _, remoteIngress := range remoteIngresses.Items {
			remoteTlsData = append(remoteTlsData, ingressTls(&remoteIngress)...)
		}
		validateTlsDataCollision(localTlsData, remoteTlsData, validation, targetDesc)
	}
	return nil
}

func validateTlsDataRegex(localTlsData []TlsDefinition, validation *objectValidation, targetDesc string) {
	for _, localTls := range localTlsData {
		validateHost(localTls.host, validation, targetDesc)
	}
}

func validateTlsDataCollision(localTlsData []TlsDefinition, remoteTlsData []TlsDefinition, validation *objectValidation, targetDesc string) {
	for _, localTls := range localTlsData {
		for _, remoteTls := range remoteTlsData {
			if localTls.host == remoteTls.host {
				//if hosts are identical rest also has to be identical
				if localTls != remoteTls {
					addViolation(
						validation,
						targetDesc,
						fmt.Sprintf("TLS collision with `%s.%s` on `%s`", remoteTls.ingressName, remoteTls.ingressNamespace, remoteTls.host),
					)
				}
			}
		}
	}
}

func validatePathDataRegex(localPathData []PathDefinition, validation *objectValidation, targetDesc string) {
	for _, localPath := range localPathData {
		validatePath(localPath.path, validation, targetDesc)
		validateHost(localPath.host, validation, targetDesc)
	}
}

func validatePathDataCollision(localPathData []PathDefinition, remotePathData []PathDefinition, validation *objectValidation, targetDesc string) {
	for _, localPath := range localPathData {
		for _, remotePath := range remotePathData {
			if localPath == remotePath {
				logger.Info(remotePath)
				violation := validationViolation{
					targetDesc,
					fmt.Sprintf("Path collision with `%s` -> `%s:%s`", remotePath.toUri(), remotePath.serviceName, remotePath.servicePort),
				}
				validation.Violations.add(violation)
			}
		}
	}
}

func ingressPath(ingress *extv1beta1.Ingress) (result []PathDefinition) {
	for _, rule := range ingress.Spec.Rules {
		host := rule.Host
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				pathDefinition := PathDefinition{
					host:        host,
					path:        path.Path,
					serviceName: path.Backend.ServiceName,
					servicePort: path.Backend.ServicePort.String(),
				}
				result = append(result, pathDefinition)
			}
		} else {
			logger.Warnf("No http definition for %v", ingress)
		}
	}
	return
}

func ingressTls(ingress *extv1beta1.Ingress) (result []TlsDefinition) {
	for _, tls := range ingress.Spec.TLS {
		for _, host := range tls.Hosts {
			tlsDefinition := TlsDefinition{host, tls.SecretName, ingress.Name, ingress.Namespace}
			result = append(result, tlsDefinition)
		}
	}
	return
}

func validateHost(host string, validation *objectValidation, targetDesc string) {
	if !ingressHostRegExp.MatchString(host) {
		addViolation(validation, targetDesc, fmt.Sprintf("Host `%s` is not valid", host))
	}
}

func validatePath(path string, validation *objectValidation, targetDesc string) {
	valid := strings.HasPrefix(path, "/")
	valid = valid && ingressPathRegExp.MatchString(path)
	if !valid {
		addViolation(validation, targetDesc, fmt.Sprintf("Path `%s` is not valid", path))
	}
}

func addViolation(validation *objectValidation, targetDesc string, message string) {
	validation.Violations.add(validationViolation{targetDesc, message})
}
