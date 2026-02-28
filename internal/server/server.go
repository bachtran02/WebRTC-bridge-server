package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/bachtran02/go-webrtc-streamer/internal/config"
	"github.com/bachtran02/go-webrtc-streamer/internal/webrtc_session"
	pb "github.com/bachtran02/go-webrtc-streamer/proto/gen/webrtc-proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type WebRTCManagerServer struct {
	pb.UnimplementedWebRTCManagerServer
	mu         sync.Mutex
	config     *config.Config
	sessionMap map[string]*StreamSession
}

func NewServer(cfg config.Config) *WebRTCManagerServer {
	return &WebRTCManagerServer{
		config:     &cfg,
		sessionMap: make(map[string]*StreamSession),
	}
}

func (s *WebRTCManagerServer) StartSession(ctx context.Context, req *pb.StartSessionRequest) (*pb.StartSessionResponse, error) {
	s.mu.Lock()

	fmt.Println("stream id", req.StreamId)

	if s.sessionMap[req.StreamId] != nil {
		/* Existing active WebRTC session */
		s.mu.Unlock()
		return &pb.StartSessionResponse{Accepted: false}, nil
	}

	s.mu.Unlock()

	/* Initialize WebRTC session */
	whipEndpoint := s.config.MediaMTX.MediaMtxHost + fmt.Sprintf("/%s/whip", req.StreamId)
	fmt.Println("whip endpoint", whipEndpoint)
	webRTCSession, err := webrtc_session.InitWebRTCSession(whipEndpoint)
	if err != nil {
		return &pb.StartSessionResponse{Accepted: false}, err
	}

	/* Create gRPC client connection */
	conn, err := grpc.NewClient(s.config.AudioProviderAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		webRTCSession.PeerConnection.Close()
		return &pb.StartSessionResponse{Accepted: false}, err
	}

	client := pb.NewAudioProviderClient(conn)
	audioStream, err := client.PullAudioStream(context.Background(), &pb.StreamRequest{StreamId: req.StreamId})
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
	s.sessionMap[req.StreamId] = newSession
	s.mu.Unlock()

	go s.runStream(req.StreamId, newSession, audioStream)
	return &pb.StartSessionResponse{Accepted: true}, nil
}

func (s *WebRTCManagerServer) StopSession(ctx context.Context, req *pb.EndSessionRequest) (*pb.EndSessionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessionMap[req.StreamId] == nil {
		return &pb.EndSessionResponse{Accepted: false}, nil
	}

	/* Stop the session and clean up resources */
	s.sessionMap[req.StreamId].Stop()
	delete(s.sessionMap, req.StreamId)

	return &pb.EndSessionResponse{Accepted: true}, nil
}
