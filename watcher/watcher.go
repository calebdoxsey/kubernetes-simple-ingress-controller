package watcher

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"github.com/bep/debounce"
	"github.com/rs/zerolog/log"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// A Payload is a collection of Kubernetes data loaded by the watcher.
type Payload struct {
	Ingresses       []IngressPayload
	TLSCertificates map[string]*tls.Certificate
}

// An IngressPayload is an ingress + its service ports.
type IngressPayload struct {
	Ingress      *networking.Ingress
	ServicePorts map[string]map[string]int
}

// A Watcher watches for ingresses in the kubernetes cluster
type Watcher struct {
	client   kubernetes.Interface
	onChange func(*Payload)
}

// New creates a new Watcher.
func New(client kubernetes.Interface, onChange func(*Payload)) *Watcher {
	return &Watcher{
		client:   client,
		onChange: onChange,
	}
}

// Run runs the watcher.
func (w *Watcher) Run(ctx context.Context) error {
	factory := informers.NewSharedInformerFactory(w.client, time.Minute)
	secretLister := factory.Core().V1().Secrets().Lister()
	serviceLister := factory.Core().V1().Services().Lister()
	ingressLister := factory.Networking().V1().Ingresses().Lister()

	addBackend := func(ingressPayload *IngressPayload, backend networking.IngressBackend) {
		svc, err := serviceLister.Services(ingressPayload.Ingress.Namespace).Get(backend.Service.Name)
		if err != nil {
			log.Error().Err(err).
				Str("namespace", ingressPayload.Ingress.Namespace).
				Str("name", backend.Service.Name).
				Msg("unknown service")
		} else {
			m := make(map[string]int)
			for _, port := range svc.Spec.Ports {
				m[port.Name] = int(port.Port)
			}
			ingressPayload.ServicePorts[svc.Name] = m
		}
	}

	onChange := func() {
		payload := &Payload{
			TLSCertificates: make(map[string]*tls.Certificate),
		}

		ingresses, err := ingressLister.List(labels.Everything())
		if err != nil {
			log.Error().Err(err).Msg("failed to list ingresses")
			return
		}

		for _, ingress := range ingresses {
			ingressPayload := IngressPayload{
				Ingress:      ingress,
				ServicePorts: make(map[string]map[string]int),
			}
			payload.Ingresses = append(payload.Ingresses, ingressPayload)

			if ingress.Spec.DefaultBackend != nil {
				addBackend(&ingressPayload, *ingress.Spec.DefaultBackend)
			}
			for _, rule := range ingress.Spec.Rules {
				if rule.HTTP != nil {
					continue
				}
				for _, path := range rule.HTTP.Paths {
					addBackend(&ingressPayload, path.Backend)
				}
			}

			for _, rec := range ingress.Spec.TLS {
				if rec.SecretName != "" {
					secret, err := secretLister.Secrets(ingress.Namespace).Get(rec.SecretName)
					if err != nil {
						log.Error().
							Err(err).
							Str("namespace", ingress.Namespace).
							Str("name", rec.SecretName).
							Msg("unknown secret")
						continue
					}

					cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
					if err != nil {
						log.Error().
							Err(err).
							Str("namespace", ingress.Namespace).
							Str("name", rec.SecretName).
							Msg("invalid tls certificate")
						continue
					}

					payload.TLSCertificates[rec.SecretName] = &cert
				}
			}
		}

		w.onChange(payload)
	}

	debounced := debounce.New(time.Second)
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			debounced(onChange)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			debounced(onChange)
		},
		DeleteFunc: func(obj interface{}) {
			debounced(onChange)
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		informer := factory.Core().V1().Secrets().Informer()
		informer.AddEventHandler(handler)
		informer.Run(ctx.Done())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		informer := factory.Networking().V1().Ingresses().Informer()
		informer.AddEventHandler(handler)
		informer.Run(ctx.Done())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		informer := factory.Core().V1().Services().Informer()
		informer.AddEventHandler(handler)
		informer.Run(ctx.Done())
		wg.Done()
	}()

	wg.Wait()
	return nil
}
