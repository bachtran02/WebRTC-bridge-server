package webrtc_session

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/pion/webrtc/v4"
)

type WebRTCSession struct {
	PeerConnection *webrtc.PeerConnection
	AudioTrack     *webrtc.TrackLocalStaticSample
}

func InitWebRTCSession(whipURL string) (s *WebRTCSession, err error) {

	mediaEngine := &webrtc.MediaEngine{}

	/* Register Opus codec with specific parameters */
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1;maxaveragebitrate=128000;cbr=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}

	/* Create the API with this engine */
	api := webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	/* Ensure peer connection is closed on error */
	defer func() {
		if err != nil {
			log.Println("Cleaning up PeerConnection due to error")
			peerConnection.Close()
		}
	}()

	log.Println("Successfully created PeerConnection!")

	/* Create local audio track sample */
	audioTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion",
	)
	if err != nil {
		return nil, err
	}

	/* Add track to PeerConnection */
	if _, err = peerConnection.AddTrack(audioTrack); err != nil {
		return nil, err
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
		return nil, err
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	if err := peerConnection.SetLocalDescription(offer); err != nil {
		return nil, err
	}
	<-gatherComplete

	log.Println("Created and set local description (offer) successfully!")

	res, err := http.Post(whipURL, "application/sdp", bytes.NewReader([]byte(peerConnection.LocalDescription().SDP)))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("WHIP failed with status: %d", res.StatusCode)
	}

	/* Apply the Answer from MediaMTX */
	answerSDP, _ := io.ReadAll(res.Body)
	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  string(answerSDP),
	}); err != nil {
		return nil, err
	}

	log.Println("Audio connection established!")

	return &WebRTCSession{
		PeerConnection: peerConnection,
		AudioTrack:     audioTrack,
	}, nil
}
