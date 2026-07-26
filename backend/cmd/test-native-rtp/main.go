package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

type message struct {
	Type            string                     `json:"type"`
	PeerID          string                     `json:"peerId,omitempty"`
	SDP             *webrtc.SessionDescription `json:"sdp,omitempty"`
	Candidate       *webrtc.ICECandidateInit   `json:"candidate,omitempty"`
	BurnInSubtitles *bool                      `json:"burnInSubtitles,omitempty"`
	AudioMode       *string                    `json:"audioMode,omitempty"`
	ChannelID       string                     `json:"channelId,omitempty"`
}

func main() {
	endpoint := flag.String("url", "ws://127.0.0.1:18088/api/ws/webrtc/16", "signaling URL")
	switchChannel := flag.String("switch", "", "switch to this channel after five seconds")
	audioMode := flag.String("audio", "both", "initial audio mode: main, sub, or both")
	restartAudio := flag.String("restart-audio", "", "restart with this audio mode after five seconds")
	subtitles := flag.Bool("subtitles", true, "enable ARIB subtitles")
	restartSubtitles := flag.String("restart-subtitles", "", "restart with ARIB subtitles true or false after media starts")
	flag.Parse()
	var restartSubtitleValue *bool
	if *restartSubtitles != "" {
		value, err := strconv.ParseBool(*restartSubtitles)
		if err != nil {
			log.Fatalf("invalid -restart-subtitles value %q", *restartSubtitles)
		}
		restartSubtitleValue = &value
	}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()
	counts := map[string]int{}
	var mu sync.Mutex
	mediaStarted := make(chan struct{})
	var mediaStartedOnce sync.Once
	var videoFrames, videoTimestampJumps int
	var subtitleMessages int
	var lastVideoTimestamp uint32
	var minVideoDelta, maxVideoDelta uint32
	var hasVideoTimestamp bool
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		for {
			packet, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			mu.Lock()
			counts[fmt.Sprint(track.Kind())]++
			if counts["video"] > 0 && counts["audio"] > 0 {
				mediaStartedOnce.Do(func() { close(mediaStarted) })
			}
			if track.Kind() == webrtc.RTPCodecTypeVideo && packet.Marker {
				if hasVideoTimestamp {
					delta := packet.Timestamp - lastVideoTimestamp
					if minVideoDelta == 0 || delta < minVideoDelta {
						minVideoDelta = delta
					}
					if delta > maxVideoDelta {
						maxVideoDelta = delta
					}
					if delta == 0 || delta > 9000 {
						videoTimestampJumps++
					}
				}
				lastVideoTimestamp = packet.Timestamp
				hasVideoTimestamp = true
				videoFrames++
			}
			mu.Unlock()
		}
	})
	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		if channel.Label() != "subtitles" {
			return
		}
		channel.OnMessage(func(_ webrtc.DataChannelMessage) {
			mu.Lock()
			subtitleMessages++
			mu.Unlock()
		})
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) { log.Printf("ICE state: %s", state) })
	u, err := url.Parse(*endpoint)
	if err != nil {
		log.Fatal(err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	var writeMu sync.Mutex
	write := func(v message) error { writeMu.Lock(); defer writeMu.Unlock(); return conn.WriteJSON(v) }
	peerID := ""
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			init := candidate.ToJSON()
			_ = write(message{Type: "ice-candidate", PeerID: peerID, Candidate: &init})
		}
	})
	if err := write(message{Type: "stream-start", BurnInSubtitles: subtitles, AudioMode: audioMode}); err != nil {
		log.Fatal(err)
	}
	remoteSet := false
	pending := make([]webrtc.ICECandidateInit, 0)
	done := make(chan error, 1)
	go func() {
		for {
			var m message
			if err := conn.ReadJSON(&m); err != nil {
				done <- err
				return
			}
			switch m.Type {
			case "offer":
				peerID = m.PeerID
				log.Printf("offer candidates=%d", strings.Count(m.SDP.SDP, "a=candidate:"))
				if err := pc.SetRemoteDescription(*m.SDP); err != nil {
					done <- err
					return
				}
				remoteSet = true
				for _, c := range pending {
					if err := pc.AddICECandidate(c); err != nil {
						done <- err
						return
					}
				}
				pending = nil
				answer, err := pc.CreateAnswer(nil)
				if err != nil {
					done <- err
					return
				}
				gathering := webrtc.GatheringCompletePromise(pc)
				if err := pc.SetLocalDescription(answer); err != nil {
					done <- err
					return
				}
				<-gathering
				if err := write(message{Type: "answer", PeerID: peerID, SDP: pc.LocalDescription()}); err != nil {
					done <- err
					return
				}
			case "ice-candidate":
				if m.Candidate == nil {
					continue
				}
				if !remoteSet {
					pending = append(pending, *m.Candidate)
					continue
				}
				if err := pc.AddICECandidate(*m.Candidate); err != nil {
					done <- err
					return
				}
			}
		}
	}()
	restartAfterMedia := func(restart func() error) {
		go func() {
			select {
			case <-mediaStarted:
				time.Sleep(time.Second)
			case <-time.After(15 * time.Second):
				return
			}
			if err := restart(); err != nil {
				select {
				case done <- err:
				default:
				}
			}
		}()
	}
	if *switchChannel != "" {
		restartAfterMedia(func() error {
			if err := write(message{Type: "restart-encoding", PeerID: peerID, ChannelID: *switchChannel, BurnInSubtitles: subtitles, AudioMode: audioMode}); err != nil {
				return err
			}
			return nil
		})
	}
	if *restartAudio != "" {
		restartAfterMedia(func() error {
			if err := write(message{Type: "restart-encoding", PeerID: peerID, BurnInSubtitles: subtitles, AudioMode: restartAudio}); err != nil {
				return err
			}
			return nil
		})
	}
	if restartSubtitleValue != nil {
		restartAfterMedia(func() error {
			return write(message{Type: "restart-encoding", PeerID: peerID, BurnInSubtitles: restartSubtitleValue, AudioMode: audioMode})
		})
	}
	waitDuration := 15 * time.Second
	if *switchChannel != "" || *restartAudio != "" || restartSubtitleValue != nil {
		waitDuration = 20 * time.Second
	}
	select {
	case err := <-done:
		log.Printf("signaling ended: %v", err)
	case <-time.After(waitDuration):
	}
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("video_rtp_packets=%d video_frames=%d video_timestamp_delta=%d..%d video_timestamp_jumps=%d audio_rtp_packets=%d subtitle_messages=%d state=%s\n",
		counts["video"], videoFrames, minVideoDelta, maxVideoDelta, videoTimestampJumps, counts["audio"], subtitleMessages, pc.ConnectionState())
	if counts["video"] == 0 || counts["audio"] == 0 {
		log.Fatal("no native WebRTC media received")
	}
	if videoTimestampJumps != 0 {
		log.Fatal("non-monotonic or discontinuous video RTP timestamps")
	}
}
