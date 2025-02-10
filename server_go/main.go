package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// Upgrade incoming HTTP connections to WebSocket.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// OfferRequest is the initial message structure from the client.
type OfferRequest struct {
	SDP           string `json:"sdp"`
	PointCloudURL string `json:"pointCloudUrl"`
}

// SignalMessage is used for all signaling messages.
type SignalMessage struct {
	Type      string                   `json:"type"`
	SDP       string                   `json:"sdp,omitempty"`
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"`
	// Optional: If you want to pass along the pointCloudUrl.
	PointCloudURL string `json:"pointCloudUrl,omitempty"`
}

func main() {
	log.Println("Starting server...")

	// Serve React build files (adjust path as needed).
	http.Handle("/", http.FileServer(http.Dir(filepath.Join("..", "build"))))

	http.Handle("/potree/", http.StripPrefix("/potree/", http.FileServer(http.Dir("potree"))))
	// WebSocket endpoint for WebRTC signaling.
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("WebSocket upgrade error:", err)
			return
		}
		defer conn.Close()
		log.Println("New WS connection from", r.RemoteAddr)

		// Create WebRTC PeerConnection.
		peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
		})
		if err != nil {
			log.Print("PeerConnection error:", err)
			return
		}

		// Create synchronization primitives
		pcReady := make(chan struct{})
		pointCloudURLChan := make(chan string, 1)

		// Create video track.
		videoTrack, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
			"potree-stream",
			"potree-video",
		)
		if err != nil {
			log.Print("Track creation error:", err)
			return
		}
		if _, err = peerConnection.AddTrack(videoTrack); err != nil {
			log.Print("AddTrack error:", err)
			return
		}

		var wg sync.WaitGroup
		wg.Add(2)

		// Signaling goroutine.
		go func() {
			defer wg.Done()
			defer close(pointCloudURLChan)
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					log.Println("WS read error:", err)
					return
				}

				var signal SignalMessage
				if err := json.Unmarshal(msg, &signal); err != nil {
					log.Println("Unmarshal error:", err)
					continue
				}

				switch signal.Type {
				case "offer":
					// Process the offer only once.
					offer := OfferRequest{
						SDP:           signal.SDP,
						PointCloudURL: signal.PointCloudURL,
					}
					handleOffer(conn, peerConnection, offer, videoTrack)
					pointCloudURLChan <- offer.PointCloudURL
					close(pcReady)
				case "candidate":
					if signal.Candidate != nil {
						if err := peerConnection.AddICECandidate(*signal.Candidate); err != nil {
							log.Println("AddICECandidate error:", err)
						}
					}
				default:
					log.Println("Unknown message type:", signal.Type)
				}
			}
		}()

		// Streaming pipeline goroutine.
		go func() {
			defer wg.Done()

			// Wait for peer connection to be ready
			<-pcReady

			// Get the point cloud URL
			pointCloudURL := <-pointCloudURLChan

			log.Println("Streaming point cloud:", pointCloudURL)

			// Start the browser stream with the received URL
			startBrowserStream(videoTrack, pointCloudURL)
		}()

		wg.Wait()
	})

	log.Fatal(http.ListenAndServe("localhost:8080", nil))
}

// addIceAttributes appends the ICE parameters if missing.
func addIceAttributes(sdp string) string {
	if !strings.Contains(sdp, "a=ice-ufrag:") {
		sdp += "\r\na=ice-ufrag:abcdefg\r\na=ice-pwd:1234567890"
	}
	return sdp
}

// handleOffer processes the incoming offer from the client.
func handleOffer(conn *websocket.Conn, pc *webrtc.PeerConnection, offer OfferRequest, track *webrtc.TrackLocalStaticSample) {
	// Ensure the SDP has ICE parameters.
	offer.SDP = addIceAttributes(offer.SDP)

	// Set the remote description.
	err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer.SDP,
	})
	if err != nil {
		log.Print("SetRemoteDescription error:", err)
		return
	}

	// Create and set the answer.
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Print("CreateAnswer error:", err)
		return
	}
	if err = pc.SetLocalDescription(answer); err != nil {
		log.Print("SetLocalDescription error:", err)
		return
	}

	// Send the answer message.
	answerMsg, _ := json.Marshal(map[string]interface{}{
		"type": "answer",
		"sdp":  pc.LocalDescription().SDP,
	})
	conn.WriteMessage(websocket.TextMessage, answerMsg)

	// Send ICE candidates as they are gathered.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			candidateJSON := c.ToJSON()
			candidateMsg, _ := json.Marshal(map[string]interface{}{
				"type":      "candidate",
				"candidate": candidateJSON,
			})
			conn.WriteMessage(websocket.TextMessage, candidateMsg)
		}
	})
}

// startBrowserStream launches Chrome headless and pipes its window capture through FFmpeg into the video track.
func startBrowserStream(track *webrtc.TrackLocalStaticSample, pointCloudURL string) {
	uniqueID := uuid.New().String()
	fullWindowTitle := "POTREE_STREAM_" + uniqueID

	// Disable headless mode and configure visible window
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Disable headless mode
		chromedp.Flag("hide-scrollbars", false),
		chromedp.Flag("window-position", "100,100"), // Position window
		chromedp.Flag("window-title", fullWindowTitle),
		chromedp.Flag("app", "http://localhost:8080/potree/viewer.html"),
		chromedp.Flag("disable-gpu", false), // Enable GPU acceleration
		chromedp.WindowSize(1280, 720),
	)

	// Force window to foreground
	opts = append(opts, chromedp.Flag("start-maximized", true))

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Add explicit window focus commands
	if err := chromedp.Run(ctx,
		chromedp.Navigate("http://localhost:8080/potree/viewer.html?pointcloudURL="+url.QueryEscape(pointCloudURL)),
		chromedp.WaitVisible(`#potree_render_area`, chromedp.ByID),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// JavaScript to ensure window focus
			return chromedp.Evaluate(`window.focus()`, nil).Do(ctx)
		}),
		chromedp.Sleep(2*time.Second), // Allow window to render
	); err != nil {
		log.Fatal("Chrome initialization failed:", err)
	}

	// err := chromedp.Run(ctx,
	// 	chromedp.Evaluate(`document.title = "`+fullWindowTitle+`"`, nil),
	// )
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// FFmpeg command for Windows screen capture
	ffmpeg := exec.Command("ffmpeg",
		"-f", "gdigrab", // Windows screen capture
		"-framerate", "30", // Capture framerate
		"-i", "title=Potree Viewer", // Capture Chrome window
		"-c:v", "libvpx", // VP8 encoding
		"-b:v", "2M", // Bitrate
		"-deadline", "realtime", // Low latency
		"-cpu-used", "4", // Faster encoding
		"-f", "rawvideo", // IVF format for VP8
		"-", // Output to stdout
	)

	// Get stdout and stderr pipes BEFORE starting
	stdout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Fatal("FFmpeg stdout pipe error:", err)
	}

	stderr, err := ffmpeg.StderrPipe()
	if err != nil {
		log.Fatal("FFmpeg stderr pipe error:", err)
	}

	if err := ffmpeg.Start(); err != nil {
		log.Fatal("FFmpeg start error:", err)
	}
	defer ffmpeg.Process.Kill()

	// Log FFmpeg stdout and stderr to console
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Print("FFmpeg stdout:", scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Print("FFmpeg stderr:", scanner.Text())
		}
	}()

	// Stream FFmpeg output to WebRTC
	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if err != nil && err != io.EOF {
			log.Print("FFmpeg read error:", err)
			break
		}
		if n == 0 {
			continue
		}

		if err := track.WriteSample(media.Sample{
			Data:     buf[:n],
			Duration: time.Second / 30,
		}); err != nil {
			log.Print("WriteSample error:", err)
		}
	}
}
