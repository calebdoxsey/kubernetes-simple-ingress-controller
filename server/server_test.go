package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/calebdoxsey/kubernetes-simple-ingress-controller/watcher"
	"github.com/stretchr/testify/assert"
	networking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, _ = context.WithTimeout(ctx, time.Second*30)

	httpPort, tlsPort := getFreePort(t), getFreePort(t)
	svcAPort := getFreePort(t)

	go func() {
		srv := &http.Server{
			Addr: fmt.Sprintf("127.0.0.1:%d", svcAPort),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "svc-a")
			}),
		}
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
		err := srv.ListenAndServe()
		if err != nil {
			t.Fatal(err)
		}
	}()

	s := New(WithHost("127.0.0.1"),
		WithPort(httpPort),
		WithTLSPort(tlsPort))
	go func() {
		err := s.Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}()

	s.Update(&watcher.Payload{
		Ingresses: []watcher.IngressPayload{
			{
				Ingress: &networking.Ingress{
					Spec: networking.IngressSpec{
						Rules: []networking.IngressRule{{
							Host: "www.example.com",
							IngressRuleValue: networking.IngressRuleValue{HTTP: &networking.HTTPIngressRuleValue{
								Paths: []networking.HTTPIngressPath{{
									Path: "/",
									Backend: networking.IngressBackend{
										Service: &networking.IngressServiceBackend{
											Name: "127.0.0.1",
											Port: networking.ServiceBackendPort{
												Number: intstr.FromString("port-a").IntVal,
											},
										},
									}},
								}},
							}},
						}},
				},
				ServicePorts: map[string]map[string]int{
					"127.0.0.1": {
						"port-a": svcAPort,
					},
				},
			},
		},
		TLSCertificates: map[string]*tls.Certificate{},
	})

	if !waitForPort(ctx, httpPort) {
		t.Fatalf("http server never started on %d", httpPort)
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", httpPort), nil)
	assert.NoError(t, err)
	req.Host = "www.example.com"
	req = req.WithContext(ctx)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	bs, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	_ = res.Body.Close()
	assert.Equal(t, "svc-a", string(bs))

}

func getFreePort(t *testing.T) int {
	li, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer li.Close()
	return li.Addr().(*net.TCPAddr).Port
}

func waitForPort(ctx context.Context, port int) bool {
	ctx, cleanup := context.WithTimeout(ctx, time.Second*10)
	defer cleanup()

	ticker := time.NewTicker(time.Millisecond * 50)
	defer ticker.Stop()

	for range ticker.C {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		if conn, err := net.Dial("tcp4", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			_ = conn.Close()
			return true
		}
	}
	panic("impossible")
}
