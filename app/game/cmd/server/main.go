package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go-zero-ddz/app/game/internal/config"
	"go-zero-ddz/app/game/internal/svc"
)

func main() {
	configFile := flag.String("f", "etc/game-local.yaml", "config file path")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Starting %s on %s:%d\n", cfg.Name, cfg.Host, cfg.Port)
	fmt.Printf("Instance ID: %s\n", cfg.InstanceId)
	fmt.Printf("Cluster mode: %v\n", cfg.Cluster.Enabled)

	// 创建服务上下文
	serviceCtx, err := svc.NewServiceContext(cfg)
	if err != nil {
		log.Fatalf("Failed to create service context: %v", err)
	}
	defer serviceCtx.Stop()

	// 启动服务
	if err := serviceCtx.Start(); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	// 等待关闭信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down...")
	serviceCtx.Stop()
	fmt.Println("Server stopped gracefully")
}
