package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeromicro/go-zero/core/logx"

	"go-zero-ddz/app/game/internal/config"
	"go-zero-ddz/app/game/internal/svc"
)

func main() {
	configFile := flag.String("f", "etc/game-local.yaml", "config file path")
	flag.Parse()

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		logx.Severef("Failed to load config: %v", err)
		return
	}

	fmt.Printf("Starting %s on %s:%d\n", cfg.Name, cfg.Host, cfg.Port)
	fmt.Printf("Instance ID: %s\n", cfg.InstanceId)
	fmt.Printf("Cluster mode: %v\n", cfg.Cluster.Enabled)

	serviceCtx, err := svc.NewServiceContext(cfg)
	if err != nil {
		logx.Severef("Failed to create service context: %v", err)
		return
	}
	defer serviceCtx.Stop()

	if err := serviceCtx.Start(); err != nil {
		logx.Severef("Failed to start service: %v", err)
		return
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down...")
	serviceCtx.Stop()
	fmt.Println("Server stopped gracefully")
}
