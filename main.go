package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/bachtran02/go-webrtc-streamer/proto/gen/webrtc-proto"
)

// server implements the WebRTCManagerServer interface
type server struct {
	pb.UnimplementedWebRTCManagerServer
}

func (s *server) StartSession(ctx context.Context, req *pb.SessionRequest) (*pb.SessionResponse, error) {
	log.Printf("Received session request: provider address: %s", req.AudioProviderAddress)

	err := createWebRTCStream(req.AudioProviderAddress)
	if err != nil {
		log.Printf("Failed to create WebRTC stream: %v", err)
		return &pb.SessionResponse{Accepted: false}, err
	}

	return &pb.SessionResponse{Accepted: true}, nil
}

func createWebRTCStream(src_address string) error {

	m := &webrtc.MediaEngine{}

	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1;maxaveragebitrate=128000;cbr=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}

	// Create the API with this engine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return err
	}

	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			log.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

	log.Println("Successfully created PeerConnection!")

	/* Create local audio track sample */
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion",
	)
	if err != nil {
		return err
	}

	/* Add track to PeerConnection */
	if _, err = peerConnection.AddTrack(audioTrack); err != nil {
		return err
	}

	/* Create an offer */
	offer, err := peerConnection.CreateOffer(&webrtc.OfferOptions{
		OfferAnswerOptions: webrtc.OfferAnswerOptions{
			VoiceActivityDetection: false,
			ICETricklingSupported:  false,
		},
		ICERestart: false,
	})
	if err != nil {
		return err
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		return err
	}
	<-gatherComplete

	log.Println("Created and set local description (offer) successfully!")

	whipURL := "http://localhost:8889/radio/whip"

	res, err := http.Post(whipURL, "application/sdp", bytes.NewReader([]byte(peerConnection.LocalDescription().SDP)))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return fmt.Errorf("WHIP failed with status: %d", res.StatusCode)
	}

	/* Apply the Answer from MediaMTX */
	answerSDP, _ := io.ReadAll(res.Body)
	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(answerSDP),
	}); err != nil {
		return err
	}

	log.Println("Audio connection established!")

	/* Connect to Kotlin server */
	conn, err := grpc.NewClient(src_address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewAudioProviderClient(conn)
	stream, err := client.PullAudioStream(context.Background(), &pb.StreamRequest{})
	if err != nil {
		return err
	}

	go func() {

		silenceOpusFrame := []byte{0xF8, 0xFF, 0xFE} // Opus silence frame

		for {
			// Receive next frame from stream (should be ready every 20ms)
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
				audioTrack.WriteSample(media.Sample{
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
	}()

	select {}
}

func main() {

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterWebRTCManagerServer(grpcServer, &server{})

	log.Println("Go gRPC server listening on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
