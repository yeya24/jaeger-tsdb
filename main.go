package main

import (
	"flag"

	"github.com/hashicorp/go-hclog"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
)

var logger = hclog.New(&hclog.LoggerOptions{
	Level:      hclog.Warn,
	Name:       "jaeger-tsdb",
	JSONFormat: true,
})

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "A path to the plugin's configuration file")
	flag.Parse()

	p, err := newStore()
	if err != nil {
		logger.Warn("error creating jaeger-tsdb", "err", err)
	}

	defer p.Close()

	grpc.Serve(p)
}
