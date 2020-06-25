package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"

	"github.com/calebdoxsey/kubernetes-simple-ingress-controller/server"
	"github.com/calebdoxsey/kubernetes-simple-ingress-controller/watcher"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	host          string
	port, tlsPort int
)

func main() {
	flag.StringVar(&host, "host", "0.0.0.0", "the host to bind")
	flag.IntVar(&port, "port", 80, "the insecure http port")
	flag.IntVar(&tlsPort, "tls-port", 443, "the secure https port")
	flag.Parse()

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	runtime.ErrorHandlers = []func(error){
		func(err error) { log.Warn().Err(err).Msg("[k8s]") },
	}

	client, err := kubernetes.NewForConfig(getKubernetesConfig())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create kubernetes client")
	}

	s := server.New(server.WithHost(host), server.WithPort(port), server.WithTLSPort(tlsPort))
	w := watcher.New(client, func(payload *watcher.Payload) {
		s.Update(payload)
	})

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		return s.Run(ctx)
	})
	eg.Go(func() error {
		return w.Run(ctx)
	})
	if err := eg.Wait(); err != nil {
		log.Fatal().Err(err).Send()
	}
}

func getKubernetesConfig() *rest.Config {
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", filepath.Join(homeDir(), ".kube", "config"))
	}
	if err != nil {
		log.Fatal().Err(err).Msg("failed to get kubernetes configuration")
	}
	return config
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
