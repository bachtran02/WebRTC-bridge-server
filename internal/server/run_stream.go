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

const (
	frameDuration = 20 * time.Millisecond
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

	// ticker := time.NewTicker(frameDuration)
	// defer ticker.Stop()

	audioTrack := session.WebRTC.AudioTrack
	ctx := session.Ctx

	silenceOpusFrame := []byte{0xF8, 0xFF, 0xFE} // Opus silence frame

	const windowSize = 100 * 30
	var (
		maxRecvDuration time.Duration
		recvWindow      [windowSize]time.Duration
		windowIdx       int
		windowTotal     time.Duration
		frameCount      int
	)

	for {
		select {
		case <-ctx.Done():
			log.Println("Stream cancelled by server context")
			return
		default:
			var data []byte

			recvStart := time.Now()
			frame, err := stream.Recv()
			d := time.Since(recvStart)

			// Update max
			if d > maxRecvDuration {
				maxRecvDuration = d
				log.Printf("stream.Recv() new max block duration: %v", maxRecvDuration)
			}

			// Update rolling average over last 100 frames
			windowTotal -= recvWindow[windowIdx]
			recvWindow[windowIdx] = d
			windowTotal += d
			windowIdx = (windowIdx + 1) % windowSize
			frameCount++
			if frameCount%windowSize == 0 {
				avg := windowTotal / windowSize
				log.Printf("[%s] Average block duration (last %d frames): %v", streamId, windowSize, avg)
			}

			if err == io.EOF {
				return // Stream closed normally
			}
			if err != nil {
				log.Printf("Error receiving audio frame: %v", err)
				return
			}

			// log.Printf("Received audio frame of size: %d bytes", len(frame.OpusData))

			if frame.IsSilence {
				data = silenceOpusFrame
			} else {
				data = frame.OpusData
			}

			err = audioTrack.WriteSample(media.Sample{
				Data:     data,
				Duration: frameDuration,
			})
			if err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
				log.Printf("Error writing to track: %v", err)
			}
		}
	}
}
