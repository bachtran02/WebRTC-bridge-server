package server

import (
	"errors"
	"io"
	"log"
	"time"

	pb "github.com/bachtran02/go-webrtc-streamer/proto/gen/webrtc-proto"
	"github.com/pion/webrtc/v4/pkg/media"
	"google.golang.org/grpc"
)

func (s *WebRTCManagerServer) runStream(streamId string, session *StreamSession, stream grpc.ServerStreamingClient[pb.AudioFrame]) {
	defer func() {
		s.mu.Lock()
		if s.sessionMap[streamId] == session {
			delete(s.sessionMap, streamId)
		}
		s.mu.Unlock()

		/* Ensure resources are cleaned up */
		session.Stop()
		log.Println("Streaming loop stopped.")
	}()

	log.Printf("Starting stream with id: %s", streamId)

	audioTrack := session.WebRTC.AudioTrack
	ctx := session.Ctx

	silenceOpusFrame := []byte{0xF8, 0xFF, 0xFE} // Opus silence frame

	for {
		select {
		case <-ctx.Done():
			log.Println("Stream cancelled by server context")
			return
		default:
			// Receive next frame from stream
			frame, err := stream.Recv()
			if err == io.EOF {
				return // Stream closed normally
			}
			if err != nil {
				log.Printf("Error receiving audio frame: %v", err)
				return
			}

			// Log received frame to the audio track
			log.Printf("Received audio frame of size: %d bytes", len(frame.OpusData))

			if frame.IsSilence {
				err = audioTrack.WriteSample(media.Sample{
					Data:     silenceOpusFrame,
					Duration: 20 * time.Millisecond,
				})
				if err != nil {
					if errors.Is(err, io.ErrClosedPipe) {
						return
					}
					log.Printf("Error writing silence to track: %v", err)
				}
			} else {
				err = audioTrack.WriteSample(media.Sample{
					Data:     frame.OpusData,
					Duration: 20 * time.Millisecond,
				})
			}

			if err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
				log.Printf("Error writing to track: %v", err)
			}
		}
	}
}
