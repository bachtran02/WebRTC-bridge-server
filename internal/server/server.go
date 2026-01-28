package server

import (
	"context"
	"sync"

	"github.com/bachtran02/go-webrtc-streamer/internal/config"
	"github.com/bachtran02/go-webrtc-streamer/internal/webrtc_session"
	pb "github.com/bachtran02/go-webrtc-streamer/proto/gen/webrtc-proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type WebRTCManagerServer struct {
	pb.UnimplementedWebRTCManagerServer
	mu             sync.Mutex
	config         *config.Config
	currentSession *StreamSession
}

func NewServer(cfg config.Config) *WebRTCManagerServer {
	return &WebRTCManagerServer{
		config:         &cfg,
		currentSession: nil,
	}
}

func (s *WebRTCManagerServer) StartSession(ctx context.Context, req *pb.StartSessionRequest) (*pb.StartSessionResponse, error) {
	s.mu.Lock()

	if s.currentSession != nil {
		/* Existing active WebRTC session */
		s.mu.Unlock()
		return &pb.StartSessionResponse{Accepted: false}, nil
	}

	s.mu.Unlock()

	/* Initialize WebRTC session */
	webRTCSession, err := webrtc_session.InitWebRTCSession(s.config.MediaMTX.WhipEndpoint)
	if err != nil {
		return &pb.StartSessionResponse{Accepted: false}, err
	}

	/* Create gRPC client connection */
	conn, err := grpc.NewClient(req.AudioProviderAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		webRTCSession.PeerConnection.Close()
		return &pb.StartSessionResponse{Accepted: false}, err
	}

	client := pb.NewAudioProviderClient(conn)
	audioStream, err := client.PullAudioStream(context.Background(), &pb.StreamRequest{})
	if err != nil {
		conn.Close()
		webRTCSession.PeerConnection.Close()
		return &pb.StartSessionResponse{Accepted: false}, err
	}

	/* Start streaming goroutine */
	streamCtx, cancel := context.WithCancel(context.Background())
	newSession := &StreamSession{
		Ctx:    streamCtx,
		Cancel: cancel,
		WebRTC: webRTCSession,
		Conn:   conn,
	}

	s.mu.Lock()
	s.currentSession = newSession
	s.mu.Unlock()

	go s.runStream(newSession, audioStream)
	return &pb.StartSessionResponse{Accepted: true}, nil
}

func (s *WebRTCManagerServer) StopSession(ctx context.Context, req *pb.EndSessionRequest) (*pb.EndSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentSession == nil {
		return &pb.EndSessionResponse{Accepted: false}, nil
	}

	/* Stop the session and clean up resources */
	s.currentSession.Stop()
	s.currentSession = nil

	return &pb.EndSessionResponse{Accepted: true}, nil
}
