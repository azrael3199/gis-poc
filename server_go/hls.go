package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
	gstCmd        *exec.Cmd
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

	// Get pointCloudUrl, viewportHeight, and viewportWidth from request parameters
	var requestBody struct {
		PointCloudURL  string `json:"pointCloudUrl"`
		ViewportHeight int    `json:"viewportHeight"`
		ViewportWidth  int    `json:"viewportWidth"`
	}
	err := json.NewDecoder(r.Body).Decode(&requestBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pointCloudUrl := requestBody.PointCloudURL
	viewportHeight := requestBody.ViewportHeight
	viewportWidth := requestBody.ViewportWidth
	ctx := openBrowser(pointCloudUrl, viewportHeight, viewportWidth)

	// Get window position and size using chromedp
	var x, y, width, height int
	// ctx, cancel := chromedp.NewContext(context.Background())

	err = chromedp.Run(ctx,
		chromedp.Evaluate(`window.screenX + window.outerWidth - window.innerWidth`, &x),
		chromedp.Evaluate(`window.screenY + window.outerHeight - window.innerHeight`, &y),
		chromedp.Evaluate(`window.innerWidth`, &width),
		chromedp.Evaluate(`window.innerHeight`, &height),
	)
	if err != nil {
		http.Error(w, "Failed to get Chrome window position", http.StatusInternalServerError)
		log.Println("Error getting Chrome window position:", err)
		return
	}

	log.Printf("Capturing window at X:%d, Y:%d, Width:%d, Height:%d\n", x, y, width, height)

	// Keep the original commented code as requested
	// err := chromedp.Run(ctx,
	// 	chromedp.Evaluate(`document.title = "`+fullWindowTitle+`"`, nil),
	// )
	// if err != nil {
	// 	log.Fatal(err)
	// }

	ffmpegCmd = exec.Command("ffmpeg",
		"-f", "dshow",
		"-i", "video=screen-capture-recorder",
		"-r", "40",
		"-vf", fmt.Sprintf("crop=%d:%d:%d:%d,format=yuv420p,scale=%d:%d", width, height, x, y+50, 1280, 720),
		"-c:v", "libvpx-vp9",
		"-b:v", "6M",
		"-g", "40",
		"-quality", "realtime",
		"-rtbufsize", "40M",
		"-speed", "6",
		"-threads", "8",
		"-deadline", "realtime",
		"-frame-parallel", "1",
		"-tile-columns", "4",
		"-row-mt", "1",
		"-hls_time", "1",
		"-hls_list_size", "5",
		"-hls_flags", "append_list+delete_segments+split_by_time",
		"-hls_segment_filename", "hls/segment_%03d.m4s",
		"-hls_segment_type", "fmp4",
		"-f", "hls",
		"hls/output.m3u8",
	)

	_, err = ffmpegCmd.StdoutPipe()
	if err != nil {
		http.Error(w, "Failed to get FFmpeg stdout", http.StatusInternalServerError)
		log.Println("Error getting FFmpeg stdout:", err)
		return
	}

	stderr, err := ffmpegCmd.StderrPipe()
	if err != nil {
		http.Error(w, "Failed to get FFmpeg stderr", http.StatusInternalServerError)
		log.Println("Error getting FFmpeg stderr:", err)
		return
	}

	// go func() {
	// 	log.Println("FFmpeg stdout:")
	// 	defer stdout.Close()
	// 	scanner := bufio.NewScanner(stdout)
	// 	for scanner.Scan() {
	// 		log.Println(scanner.Text())
	// 	}
	// }()

	go func() {
		log.Println("FFmpeg stderr:")
		defer stderr.Close()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}
	}()

	if err := ffmpegCmd.Start(); err != nil {
		http.Error(w, "Failed to start stream", http.StatusInternalServerError)
		log.Println("FFmpeg error:", err)
		return
	}

	log.Println("Streaming started")
	w.Write([]byte("Stream started"))
}

// // stopStream stops the FFmpeg process
// func stopStream(w http.ResponseWriter, r *http.Request) {
// 	mu.Lock()
// 	defer mu.Unlock()

// 	if ffmpegCmd == nil {
// 		http.Error(w, "No active stream", http.StatusNotFound)
// 		return
// 	}

// 	if err := ffmpegCmd.Process.Kill(); err != nil {
// 		http.Error(w, "Failed to stop stream", http.StatusInternalServerError)
// 		log.Println("Error stopping FFmpeg:", err)
// 		return
// 	}

// 	ffmpegCmd = nil

// 	closeBrowser()

// 	// Clear the contents of the /hls/ folder
// 	hlsDir := "hls/"
// 	dir, err := os.Open(hlsDir)
// 	if err != nil {
// 		log.Println("Error opening HLS directory:", err)
// 	} else {
// 		defer dir.Close()
// 		files, err := dir.Readdirnames(-1)
// 		if err != nil {
// 			log.Println("Error reading HLS directory:", err)
// 		} else {
// 			for _, file := range files {
// 				err := os.Remove(hlsDir + file)
// 				if err != nil {
// 					log.Println("Error removing file:", err)
// 				}
// 			}
// 		}
// 	}

// 	log.Println("Streaming stopped")
// 	w.Write([]byte("Stream stopped"))
// }

// func startStream(w http.ResponseWriter, r *http.Request) {
// 	mu.Lock()
// 	defer mu.Unlock()

// 	if gstCmd != nil {
// 		http.Error(w, "Stream already running", http.StatusConflict)
// 		return
// 	}

// 	os.MkdirAll("hls", os.ModePerm)

// 	var requestBody struct {
// 		PointCloudURL string `json:"pointCloudUrl"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
// 		http.Error(w, err.Error(), http.StatusBadRequest)
// 		return
// 	}

// 	pointCloudUrl := requestBody.PointCloudURL
// 	go openBrowser(pointCloudUrl)

// 	gstCmd = exec.Command("gst-launch-1.0",
// 		"d3d11screencapturesrc", "!", "videoconvert", "!", "videoscale", "!",
// 		"video/x-raw,framerate=40/1,width=1920,height=1080", "!", "vp9enc",
// 		"target-bitrate=6000000", "cpu-used=6", "deadline=1", "threads=8", "tile-columns=4", "row-mt=true", "!",
// 		"filesink", "location=NUL",
// 	)

// 	// gstCmd.Stdout = os.Stdout
// 	// gstCmd.Stderr = os.Stderr

// 	ffmpegCmd = exec.Command("ffmpeg",
// 		"-i", "-",
// 		"-c:v", "libvpx-vp9",
// 		"-c:a", "opus",
// 		"-hls_time", "1",
// 		"-hls_list_size", "5",
// 		"-hls_flags", "append_list+delete_segments+split_by_time",
// 		"-hls_segment_filename", "hls/segment_%03d.m4s",
// 		"-hls_segment_type", "fmp4",
// 		"-f", "hls",
// 		"-loglevel", "debug",
// 	)

// 	ffmpegIn, err := ffmpegCmd.StdinPipe()
// 	if err != nil {
// 		http.Error(w, "Failed to get FFmpeg stdin", http.StatusInternalServerError)
// 		log.Println("Error getting FFmpeg stdin:", err)
// 		return
// 	}

// 	gstOut, err := gstCmd.StdoutPipe()
// 	if err != nil {
// 		http.Error(w, "Failed to get GStreamer stdout", http.StatusInternalServerError)
// 		log.Println("Error getting GStreamer stdout:", err)
// 		return
// 	}

// 	gstErr, err := gstCmd.StderrPipe()
// 	if err != nil {
// 		http.Error(w, "Failed to get GStreamer stderr", http.StatusInternalServerError)
// 		log.Println("Error getting GStreamer stderr:", err)
// 		return
// 	}

// 	go func() {
// 		defer ffmpegIn.Close()
// 		log.Println("Copying from GStreamer to FFmpeg...")
// 		_, err := io.Copy(ffmpegIn, gstOut)
// 		if err != nil {
// 			log.Println("Error copying from GStreamer to FFmpeg:", err)
// 		}
// 	}()

// 	go func() {
// 		log.Println("GStreamer stderr:")
// 		defer gstErr.Close()
// 		scanner := bufio.NewScanner(gstErr)
// 		for scanner.Scan() {
// 			log.Println(scanner.Text())
// 		}
// 	}()

// 	if err := ffmpegCmd.Start(); err != nil {
// 		http.Error(w, "Failed to start FFmpeg", http.StatusInternalServerError)
// 		log.Println("FFmpeg error:", err)
// 		return
// 	}

// 	if err := gstCmd.Start(); err != nil {
// 		http.Error(w, "Failed to start GStreamer", http.StatusInternalServerError)
// 		log.Println("GStreamer error:", err)
// 		return
// 	}

// 	log.Println("Streaming started")
// 	w.Write([]byte("Stream started"))
// }

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
func openBrowser(pointCloudUrl string, viewportHeight int, viewportWidth int) context.Context {

	// Disable headless mode and configure visible window
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // Disable headless mode
		chromedp.Flag("hide-scrollbars", false),
		chromedp.Flag("window-position", "0,0"), // Position window
		// chromedp.Flag("window-title", fullWindowTitle),
		chromedp.Flag("app", "http://localhost:8080/potree/viewer.html?pointcloudURL="+url.QueryEscape(pointCloudUrl)),
		chromedp.Flag("disable-gpu", false), // Enable GPU acceleration
		chromedp.Flag("disable-infobars", true),
		chromedp.WindowSize(viewportWidth, viewportHeight),
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

	return browserCtx
}

// closeBrowser closes the Chromedp session
func closeBrowser() {
	if browserCancel != nil {
		browserCancel() // Cancels the Chrome context
		log.Println("Chrome closed")
	}
}
