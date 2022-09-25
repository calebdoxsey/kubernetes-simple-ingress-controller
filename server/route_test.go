package server

import (
	"crypto/tls"
	"net/url"
	"testing"

	"github.com/calebdoxsey/kubernetes-simple-ingress-controller/watcher"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestRoutingTable(t *testing.T) {
	t.Run("empty payload", func(t *testing.T) {
		rt := NewRoutingTable(nil)
		u, err := rt.GetBackend("host", "/")
		assert.Nil(t, u)
		assert.Error(t, err)

		cert, err := rt.GetCertificate("host")
		assert.Nil(t, cert)
		assert.Error(t, err)
	})
	t.Run("default backend with no rules", func(t *testing.T) {
		rt := NewRoutingTable(&watcher.Payload{
			Ingresses: []watcher.IngressPayload{{
				Ingress: &networking.Ingress{Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "example.default.svc.cluster.local",
							Port: networking.ServiceBackendPort{
								Number: 80,
							},
						}},
				}},
			}},
		})
		u, err := rt.GetBackend("www.example.com", "/users/1234")
		assert.Error(t, err)
		assert.Nil(t, u)
	})
	t.Run("default backend with host rule", func(t *testing.T) {
		rt := NewRoutingTable(&watcher.Payload{
			Ingresses: []watcher.IngressPayload{{
				Ingress: &networking.Ingress{Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "example",
							Port: networking.ServiceBackendPort{
								Number: 80,
							},
						},
					},
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
					}},
				}},
			}},
		},
		)
		u, err := rt.GetBackend("www.example.com:8443", "/users/1234")
		assert.NoError(t, err)
		assert.Equal(t, &url.URL{
			Scheme: "http",
			Host:   "example:80",
		}, u)
	})
	t.Run("default backend with named port", func(t *testing.T) {
		rt := NewRoutingTable(&watcher.Payload{
			Ingresses: []watcher.IngressPayload{{
				Ingress: &networking.Ingress{Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "example",
							Port: networking.ServiceBackendPort{
								Number: intstr.FromString("http").IntVal,
							},
						},
					},
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
					}},
				}},
				ServicePorts: map[string]map[string]int{
					"example": {"http": 80},
				},
			}},
		})
		u, err := rt.GetBackend("www.example.com", "/users/1234")
		assert.NoError(t, err)
		assert.Equal(t, &url.URL{
			Scheme: "http",
			Host:   "example:80",
		}, u)
	})
	t.Run("tls cert", func(t *testing.T) {
		cert1 := new(tls.Certificate)
		rt := NewRoutingTable(&watcher.Payload{
			Ingresses: []watcher.IngressPayload{{
				Ingress: &networking.Ingress{Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "example.default.svc.cluster.local",
							Port: networking.ServiceBackendPort{
								Number: 80,
							},
						},
					},
					TLS: []networking.IngressTLS{{
						Hosts:      []string{"www.example.com"},
						SecretName: "example",
					}},
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
					}},
				}},
			}},
			TLSCertificates: map[string]*tls.Certificate{
				"example": cert1,
			},
		})
		cert, err := rt.GetCertificate("www.example.com")
		assert.NoError(t, err)
		assert.Equal(t, cert, cert1)
	})
	t.Run("wildcard tls cert", func(t *testing.T) {
		cert1 := new(tls.Certificate)
		rt := NewRoutingTable(&watcher.Payload{
			Ingresses: []watcher.IngressPayload{{
				Ingress: &networking.Ingress{Spec: networking.IngressSpec{
					DefaultBackend: &networking.IngressBackend{
						Service: &networking.IngressServiceBackend{
							Name: "example.default.svc.cluster.local",
							Port: networking.ServiceBackendPort{
								Number: 80,
							},
						},
					},
					TLS: []networking.IngressTLS{{
						Hosts:      []string{"*.example.com"},
						SecretName: "example",
					}},
					Rules: []networking.IngressRule{{
						Host: "www.example.com",
					}},
				}},
			}},
			TLSCertificates: map[string]*tls.Certificate{
				"example": cert1,
			},
		})
		cert, err := rt.GetCertificate("www.example.com")
		assert.NoError(t, err)
		assert.Equal(t, cert, cert1)
	})
}
