package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/grpc"

	"github.com/bachtran02/go-webrtc-streamer/internal/config"
	"github.com/bachtran02/go-webrtc-streamer/internal/server"
	pb "github.com/bachtran02/go-webrtc-streamer/proto/gen/webrtc-proto"
)

func main() {

	cfgPath := flag.String("config", "config.yml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		log.Printf("failed to load config: %v\n", err)
		os.Exit(-1)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Grpc.Host, cfg.Grpc.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	webRTCServerManager := server.NewServer(cfg)
	pb.RegisterWebRTCManagerServer(grpcServer, webRTCServerManager)

	log.Printf("Go gRPC server listening on %s:%d", cfg.Grpc.Host, cfg.Grpc.Port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
