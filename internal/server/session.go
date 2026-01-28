package server

import (
	"context"

	"github.com/bachtran02/go-webrtc-streamer/internal/webrtc_session"
	"google.golang.org/grpc"
)

type StreamSession struct {
	Ctx    context.Context
	Cancel context.CancelFunc

	// Resources used by this session
	WebRTC *webrtc_session.WebRTCSession
	Conn   *grpc.ClientConn
}

// Stop cleans up all resources for this specific session
func (s *StreamSession) Stop() {
	if s.Cancel != nil {
		s.Cancel()
	}
	if s.WebRTC != nil && s.WebRTC.PeerConnection != nil {
		s.WebRTC.PeerConnection.Close()
	}
	if s.Conn != nil {
		s.Conn.Close()
	}
}
