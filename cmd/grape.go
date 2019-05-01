package main

import (
	"flag"
	"os"

	"github.com/txn2/grape"
	"github.com/txn2/micro"
)

var (
	elasticsearchHostEnv      = getEnv("ELASTICSEARCH_HOST", "elasticsearch:9200")
	elasticsearchSchemeEnv    = getEnv("ELASTICSEARCH_SCHEME", "http")
	elasticsearchProxyPathEnv = getEnv("ELASTICSEARCH_PROXY_PATH", "/es")

	// Grape uses the txn2/provision service to authenticate Account keys used for
	// the Elastisearch data source. See: https://github.com/txn2/provision
	provisionServiceEnv = getEnv("PROVISION_SERVICE", "http://api-provision:8070")
)

func main() {

	elasticsearchProxyPath := flag.String("elasticsearchProxyPath", elasticsearchProxyPathEnv, "Elasticsearch proxy path.")
	elasticsearchHost := flag.String("elasticsearchHost", elasticsearchHostEnv, "Elasticsearch host.")
	elasticsearchScheme := flag.String("elasticsearchScheme", elasticsearchSchemeEnv, "Elasticsearch scheme.")
	provisionService := flag.String("provisionService", provisionServiceEnv, "Provision service.")

	serverCfg, _ := micro.NewServerCfg("Grape")
	server := micro.NewServer(serverCfg)

	auth := grape.NewAuth(grape.Cfg{
		HttpClient:       server.Client.Http,
		Logger:           server.Logger,
		PathPrefix:       *elasticsearchProxyPath,
		ProvisionService: *provisionService,
	})

	proxyPath := server.Router.Group(*elasticsearchProxyPath, auth.RequestHandler)

	proxyPath.Any("/*any",
		server.ReverseProxy(micro.PxyCfg{
			Scheme: elasticsearchScheme,
			Host:   elasticsearchHost,
			Strip:  elasticsearchProxyPath,
		}),
	)

	// run provisioning server
	server.Run()
}

// getEnv gets an environment variable or sets a default if
// one does not exist.
func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}

	return value
}
