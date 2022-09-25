package server

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/calebdoxsey/kubernetes-simple-ingress-controller/watcher"
	"github.com/rs/zerolog/log"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const BackendProtocolAnnotation = "kubernetes-simple-ingress-controller/backend-protocol"

// A RoutingTable contains the information needed to route a request.
type RoutingTable struct {
	certificatesByHost map[string]map[string]*tls.Certificate
	backendsByHost     map[string][]routingTableBackend
}

type routingTableBackend struct {
	pathRE *regexp.Regexp
	url    *url.URL
}

func newRoutingTableBackend(scheme string, path string, serviceName string, servicePort int) (routingTableBackend, error) {
	rtb := routingTableBackend{
		url: &url.URL{
			Scheme: scheme,
			Host:   fmt.Sprintf("%s:%d", serviceName, servicePort),
		},
	}
	var err error
	if path != "" {
		rtb.pathRE, err = regexp.Compile(path)
	}
	return rtb, err
}

func (rtb routingTableBackend) matches(path string) bool {
	if rtb.pathRE == nil {
		return true
	}
	return rtb.pathRE.MatchString(path)
}

// NewRoutingTable creates a new RoutingTable.
func NewRoutingTable(payload *watcher.Payload) *RoutingTable {
	rt := &RoutingTable{
		certificatesByHost: make(map[string]map[string]*tls.Certificate),
		backendsByHost:     make(map[string][]routingTableBackend),
	}
	rt.init(payload)
	return rt
}

func (rt *RoutingTable) init(payload *watcher.Payload) {
	if payload == nil {
		return
	}
	for _, ingressPayload := range payload.Ingresses {
		for _, rule := range ingressPayload.Ingress.Spec.Rules {
			m, ok := rt.certificatesByHost[rule.Host]
			if !ok {
				m = make(map[string]*tls.Certificate)
				rt.certificatesByHost[rule.Host] = m
			}
			for _, t := range ingressPayload.Ingress.Spec.TLS {
				for _, h := range t.Hosts {
					cert, ok := payload.TLSCertificates[t.SecretName]
					if ok {
						m[h] = cert
					}
				}
			}
			rt.addBackend(ingressPayload, rule)
		}
	}
}

func (rt *RoutingTable) addBackend(ingressPayload watcher.IngressPayload, rule networking.IngressRule) {
	scheme, ok := ingressPayload.Ingress.Annotations[BackendProtocolAnnotation]
	if !ok {
		scheme = "http"
	}
	scheme = strings.ToLower(scheme)

	if rule.HTTP == nil {
		if ingressPayload.Ingress.Spec.DefaultBackend != nil {
			backend := ingressPayload.Ingress.Spec.DefaultBackend
			rtb, err := newRoutingTableBackend(scheme, "", backend.Service.Name,
				rt.getServicePort(ingressPayload, backend.Service.Name, intstr.FromInt(int(backend.Service.Port.Number))))
			if err != nil {
				// this shouldn't happen
				log.Error().Err(err).Send()
				return
			}
			rt.backendsByHost[rule.Host] = append(rt.backendsByHost[rule.Host], rtb)
		}
	} else {
		for _, path := range rule.HTTP.Paths {
			backend := path.Backend
			rtb, err := newRoutingTableBackend(scheme, path.Path, backend.Service.Name,
				rt.getServicePort(ingressPayload, backend.Service.Name, intstr.FromInt(int(backend.Service.Port.Number))))
			if err != nil {
				log.Error().Err(err).Interface("path", path).Msg("invalid ingress rule path regex")
				continue
			}
			rt.backendsByHost[rule.Host] = append(rt.backendsByHost[rule.Host], rtb)
		}
	}
}

func (rt *RoutingTable) getServicePort(ingressPayload watcher.IngressPayload, serviceName string, servicePort intstr.IntOrString) int {
	if servicePort.Type == intstr.Int {
		return servicePort.IntValue()
	}
	if m, ok := ingressPayload.ServicePorts[serviceName]; ok {
		return m[servicePort.String()]
	}
	return 80
}

func (rt *RoutingTable) matches(sni string, certHost string) bool {
	for strings.HasPrefix(certHost, "*.") {
		if idx := strings.IndexByte(sni, '.'); idx >= 0 {
			sni = sni[idx+1:]
		} else {
			return false
		}
		certHost = certHost[2:]
	}
	return sni == certHost
}

// GetCertificate gets a certificate.
func (rt *RoutingTable) GetCertificate(sni string) (*tls.Certificate, error) {
	hostCerts, ok := rt.certificatesByHost[sni]
	if ok {
		for h, cert := range hostCerts {
			if rt.matches(sni, h) {
				return cert, nil
			}
		}
	}
	return nil, fmt.Errorf("certificate not found for %s", sni)
}

// GetBackend gets the backend for the given host and path.
func (rt *RoutingTable) GetBackend(host, path string) (*url.URL, error) {
	if idx := strings.IndexByte(host, ':'); idx > 0 {
		host = host[:idx]
	}
	backends := rt.backendsByHost[host]
	for _, backend := range backends {
		if backend.matches(path) {
			return backend.url, nil
		}
	}
	return nil, errors.New("backend not found")
}
