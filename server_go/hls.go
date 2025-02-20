package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rs/cors"
)

var (
	ffmpegCmd     *exec.Cmd
	mu            sync.Mutex
	browserCtx    context.Context
	browserCancel context.CancelFunc
)

func main() {
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders: []string{"*"},
	})

	// Mux for routing
	mux := http.NewServeMux()

	// Serve HLS files
	mux.Handle("/file/", http.StripPrefix("/file/", http.FileServer(http.Dir("data"))))
	mux.Handle("/potree/", http.StripPrefix("/potree/", http.FileServer(http.Dir("potree"))))
	mux.Handle("/hls/", http.StripPrefix("/hls/", http.FileServer(http.Dir("hls"))))

	// API routes
	mux.HandleFunc("/start", startStream)
	mux.HandleFunc("/stop", stopStream)

	log.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", c.Handler(mux)))
}

// startStream starts FFmpeg to capture video and output HLS
func startStream(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if ffmpegCmd != nil {
		http.Error(w, "Stream already running", http.StatusConflict)
		return
	}

	// Ensure HLS directory exists
	os.MkdirAll("hls", os.ModePerm)

	// Get pointCloudUrl from request parameters
	var requestBody struct {
		PointCloudURL string `json:"pointCloudUrl"`
	}
	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pointCloudUrl := requestBody.PointCloudURL

	go openBrowser(pointCloudUrl)

	// err := chromedp.Run(ctx,
	// 	chromedp.Evaluate(`document.title = "`+fullWindowTitle+`"`, nil),
	// )
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// FFmpeg command to capture screen and generate HLS output
	ffmpegCmd = exec.Command("ffmpeg",
		"-f", "dshow",
		"-framerate", "30",
		"-i", "video=screen-capture-recorder",
		"-vf", "scale=1920:1080",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency", // Low-latency optimization
		"-b:v", "2M",
		"-g", "30", // Keyframe interval
		"-hls_time", "1", // Each segment is 2 seconds
		"-hls_list_size", "5", // Keep 10 segments in the playlist
		"-hls_flags", "append_list", // Append new segments instead of deleting
		"-hls_flags", "delete_segments+append_list+split_by_time", // Each segment is standalone
		"-hls_segment_filename", "hls/segment_%03d.ts", // Save segments properly
		"-hls_segment_type", "mpegts",
		"-hls_flags", "delete_segments", // Remove old segments
		"-f", "hls",
		"hls/output.m3u8",
	)

	if err := ffmpegCmd.Start(); err != nil {
		http.Error(w, "Failed to start stream", http.StatusInternalServerError)
		log.Println("FFmpeg error:", err)
		return
	}

	log.Println("Streaming started")
	w.Write([]byte("Stream started"))
}

// stopStream stops the FFmpeg process
func stopStream(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if ffmpegCmd == nil {
		http.Error(w, "No active stream", http.StatusNotFound)
		return
	}

	if err := ffmpegCmd.Process.Kill(); err != nil {
		http.Error(w, "Failed to stop stream", http.StatusInternalServerError)
		log.Println("Error stopping FFmpeg:", err)
		return
	}

	ffmpegCmd = nil

	closeBrowser()

	// Clear the contents of the /hls/ folder
	hlsDir := "hls/"
	dir, err := os.Open(hlsDir)
	if err != nil {
		log.Println("Error opening HLS directory:", err)
	} else {
		defer dir.Close()
		files, err := dir.Readdirnames(-1)
		if err != nil {
			log.Println("Error reading HLS directory:", err)
		} else {
			for _, file := range files {
				err := os.Remove(hlsDir + file)
				if err != nil {
					log.Println("Error removing file:", err)
				}
			}
		}
	}

	log.Println("Streaming stopped")
	w.Write([]byte("Stream stopped"))
}

// openBrowser launches Chrome using chromedp
func openBrowser(pointCloudUrl string) {

	// Disable headless mode and configure visible window
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Disable headless mode
		chromedp.Flag("hide-scrollbars", false),
		chromedp.Flag("window-position", "100,100"), // Position window
		// chromedp.Flag("window-title", fullWindowTitle),
		chromedp.Flag("app", "http://localhost:8080/potree/viewer.html?pointcloudURL="+url.QueryEscape(pointCloudUrl)),
		chromedp.Flag("disable-gpu", false), // Enable GPU acceleration
		chromedp.WindowSize(1280, 720),
	)

	// Force window to foreground
	opts = append(opts, chromedp.Flag("start-maximized", true))

	browserCtx, browserCancel = chromedp.NewExecAllocator(context.Background(),
		opts...,
	)

	// Create new Chrome context
	browserCtx, browserCancel = chromedp.NewContext(browserCtx)

	// Add explicit window focus commands
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate("http://localhost:8080/potree/viewer.html?pointcloudURL="+url.QueryEscape(pointCloudUrl)),
		// chromedp.WaitVisible(`#potree_render_area`, chromedp.ByID),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// JavaScript to ensure window focus
			return chromedp.Evaluate(`window.focus()`, nil).Do(ctx)
		}),
		// chromedp.ActionFunc(func(ctx context.Context) error {
		// 	// JavaScript to ensure window focus
		// 	return chromedp.Evaluate(`document.title = "`+fullWindowTitle+`"`, nil).Do(ctx)
		// }),
		chromedp.Sleep(2*time.Second), // Allow window to render
	); err != nil {
		log.Fatal("Chrome initialization failed:", err)
	}
}

// closeBrowser closes the Chromedp session
func closeBrowser() {
	if browserCancel != nil {
		browserCancel() // Cancels the Chrome context
		log.Println("Chrome closed")
	}
}
