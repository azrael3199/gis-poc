import express from "express";
import cors from "cors";
import path from "path";
import { fileURLToPath } from "url";
import { Browser, Page } from "puppeteer";
import { getStream, launch } from "puppeteer-stream";
import wrtc from "@roamhq/wrtc";
import dgram from "dgram";
import type internal from "stream";
import { spawn, type ChildProcessWithoutNullStreams } from "child_process";
import { v4 } from "uuid";
import { WebSocketServer } from "ws";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();

app.use(cors({ origin: "*" }));
app.use("/potree", express.static(path.join(__dirname, "potree")));
app.use("/data", express.static(path.join(__dirname, "data")));

// let browser: Browser | null = null;
// let page: Page | null = null;
// const peerConnections: Record<string, any> = {};
// interface Client {
//   id: string;
//   peerConnection: wrtc.RTCPeerConnection;
//   browser: Browser;
//   page: Page;
//   ffmpeg: ChildProcessWithoutNullStreams;
// }

// const clients: Map<string, Client> = new Map();

async function startStream(
  page: Page,
  videoSrc: wrtc.nonstandard.RTCVideoSource,
  options: {
    width: number;
    height: number;
  },
  stream: internal.Transform
) {
  const ffmpeg = spawn("ffmpeg", [
    "-f",
    "webm", // Input format
    "-i",
    "pipe:0", // Read from piped Puppeteer stream
    "-an", // Disable audio
    "-r",
    "60",
    "-vf",
    "scale=1280:720", // Set resolution
    // '-c:v',
    // 'libx264', // Hardware accelerated H.264 encoding (if using NVIDIA GPU)
    "-preset",
    "fast",
    "-deadline",
    "realtime",
    "-tune",
    "zerolatency",
    "-pix_fmt",
    "yuv420p", // Set pixel format to YUV420p
    "-f",
    "flv", // Output as raw video
    "-f",
    "rawvideo", // Output as raw video
    "pipe:1",
  ]);

  ffmpeg.on("error", (error) => {
    console.error(`FFmpeg error: ${error.message}`);
  });

  ffmpeg.stderr.on("data", (data) => {
    console.error(`FFmpeg stderr: ${data}`);
  });

  // Pipe Puppeteer stream into FFmpeg
  stream.pipe(ffmpeg.stdin);

  // Calculate expected frame size (YUV420)
  const frameSize = options.width * options.height * 1.5;
  let buffer = Buffer.alloc(0);

  ffmpeg.stdout.on("data", (data) => {
    buffer = Buffer.concat([buffer, data]);

    // Process all complete frames in the buffer
    while (buffer.length >= frameSize) {
      const frame = buffer.subarray(0, frameSize);
      buffer = buffer.subarray(frameSize);

      // console.log(`Sending frame of size ${frame.length}`);

      // Send the raw YUV420 frame to the video source
      videoSrc.onFrame({
        data: new Uint8Array(frame),
        width: options.width,
        height: options.height,
      });
    }
  });
}

// app.post("/webrtc-offer", async (req, res) => {
//   try {
//     const { sdp, pointCloudUrl } = req.body;

//     // Close existing browser instance
//     if (browser) {
//       await browser.close();
//     }

//     // Launch browser and navigate to Potree viewer
//     browser = await launch({
//       defaultViewport: null,
//       executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe",
//       headless: false, // Show the browser
//       args: [
//         "--no-sandbox",
//         "--disable-setuid-sandbox",
//         "--use-gl=egl", // Enable OpenGL ES via EGL
//         "--enable-webgl",
//         "--ignore-gpu-blocklist",
//         "--allow-insecure-localhost",
//         "--disable-gpu-sandbox",
//         "--enable-logging",
//         "--v=1",
//         "--enable-unsafe-swiftshader",
//       ],
//     });
//     console.log("🟢 Puppeteer browser launched");

//     const page = await browser?.newPage();
//     await page?.setViewport({ width: 1980, height: 1080 });

//     // Enable console logging from page
//     page?.on("console", (msg) => console.log("Page log:", msg.text()));
//     page?.on("pageerror", (err) => console.error("Page error:", err));

//     await page?.goto(
//       `http://localhost:3000/potree/viewer.html?pointcloudURL=${pointCloudUrl}`,
//       {
//         waitUntil: "domcontentloaded",
//         timeout: 60000,
//       }
//     );

//     // Create WebRTC peer connection
//     const peerConnection = new wrtc.RTCPeerConnection({
//       iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
//     });

//     // Use RTCVideoSource to create a video track
//     const { RTCVideoSource } = wrtc.nonstandard;
//     const videoSource = new RTCVideoSource();
//     const videoTrack = videoSource.createTrack();

//     // Get the compressed video stream from the page
//     const stream = await getStream(page, { audio: false, video: true });

//     console.log("🟢 WebRTC peer connection created, adding track");
//     peerConnection.addTrack(videoTrack);

//     const ffmpeg = await startStream(
//       // @ts-expect-error browser and page are initialized later
//       page,
//       videoSource,
//       {
//         videoBitrate: "1000k",
//         keyframeInterval: 1,
//         width: 1280,
//         height: 720,
//       },
//       stream
//     );
//     // WebRTC signaling
//     peerConnection.onicecandidate = (event) => {
//       if (event.candidate) {
//         console.log("New ICE candidate:", event.candidate);
//       }
//     };

//     const offer = new wrtc.RTCSessionDescription({
//       type: "offer",
//       sdp: sdp,
//     });

//     await peerConnection.setRemoteDescription(offer);
//     const answer = await peerConnection.createAnswer();
//     await peerConnection.setLocalDescription(answer);

//     peerConnections[req.ip] = peerConnection;

//     const clientId = v4();
//     const client = {
//       id: clientId,
//       peerConnection,
//       browser,
//       page,
//       ffmpeg,
//     };
//     clients.set(clientId, client);

//     return res.json({
//       sdp: peerConnection.localDescription,
//     });
//   } catch (error) {
//     console.error("WebRTC offer error:", error);
//     return res.status(500).json({
//       error: error instanceof Error ? error.message : String(error),
//       stack: error instanceof Error ? error.stack : undefined,
//     });
//   }
// });

// // Graceful shutdown
// process.on("SIGINT", async () => {
//   if (browser) {
//     await browser.close();
//   }
//   process.exit();
// });

const server = app.listen(3000, () =>
  console.log("Server running on port 3000")
);
// WebSocket setup for streaming
const wss = new WebSocketServer({ server });

interface Client {
  id: string;
  ws: WebSocket;
  peerConnection: wrtc.RTCPeerConnection;
  browser: Browser;
  page: Page;
  ffmpeg: ChildProcessWithoutNullStreams;
}

const clients: Map<string, Client> = new Map();

// Spawn FFmpeg to decode the stream
// const ffmpeg = spawn('ffmpeg', [
//   '-f',
//   'webm', // Input format
//   '-i',
//   'pipe:0', // Read from piped Puppeteer stream
//   '-an', // Disable audio
//   '-r',
//   '60',
//   '-vf',
//   'scale=1280:720', // Set resolution
//   // '-c:v',
//   // 'libx264', // Hardware accelerated H.264 encoding (if using NVIDIA GPU)
//   '-preset',
//   'fast',
//   '-tune',
//   'zerolatency',
//   '-pix_fmt',
//   'yuv420p', // Set pixel format to YUV420p
//   '-f',
//   'flv', // Output as raw video
//   '-f',
//   'rawvideo', // Output as raw video
//   'pipe:1',
// ]);

wss.on("connection", (ws) => {
  const clientId = v4();
  console.log(`New client connected: ${clientId}`);

  let client: Client;

  ws.on("message", async (message: string) => {
    const data = JSON.parse(message);

    if (data.type === "offer") {
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { sdp, pointCloudUrl } = data;

      // Create RTCPeerConnection
      const peerConnection = new wrtc.RTCPeerConnection();

      // Handle ICE candidates from the client
      peerConnection.onicecandidate = (event) => {
        // console.log('ICE candidate event', event);
        if (event.candidate) {
          ws.send(
            JSON.stringify({ type: "candidate", candidate: event.candidate })
          );
        }
      };

      // Handle track event (if needed)
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      peerConnection.ontrack = (event: RTCTrackEvent) => {
        // Handle incoming tracks if applicable
      };

      // Launch Puppeteer browser
      const browser = await launch({
        defaultViewport: { width: 1280, height: 720 },
        executablePath: "C:/Program Files/Google/Chrome/Application/chrome.exe",
        // headless: true, // Show the browser
        args: [
          "--no-sandbox",
          "--disable-setuid-sandbox",
          "--use-gl=egl", // Enable OpenGL ES via EGL
          "--enable-webgl",
          "--ignore-gpu-blocklist",
          "--allow-insecure-localhost",
          "--disable-gpu-sandbox",
          "--enable-logging",
          "--v=1",
          "--enable-unsafe-swiftshader",
        ],
      });

      console.log("🟢 Puppeteer browser launched");

      const page = await browser.newPage();
      await page.setViewport({ width: 1280, height: 720 });

      page.on("requestfailed", (request) => {
        console.log(
          `Request failed: ${request.url()} - ${request.failure()?.errorText}`
        );
      });

      // Navigate to the Cesium viewer page with fileId
      await page.goto(
        `http://localhost:3000/potree/viewer.html?pointcloudURL=${pointCloudUrl}`
      );

      // Use RTCVideoSource to create a video track
      const { RTCVideoSource } = wrtc.nonstandard;
      const videoSource = new RTCVideoSource();
      const videoTrack = videoSource.createTrack();

      // Add the video track to the peer connection
      peerConnection.addTrack(videoTrack);

      // Get the compressed video stream from the page
      const stream = await getStream(page, { audio: false, video: true });

      const ffmpeg = await startStream(
        // @ts-expect-error browser and page are initialized later
        page,
        videoSource,
        {
          videoBitrate: "1000k",
          keyframeInterval: 1,
          width: 1280,
          height: 720,
        },
        stream
      );
      // let buffer = Buffer.alloc(0);
      // const frameSize = 1280 * 720 * 1.5;
      // stream.on("data", (data) => {
      //   buffer = Buffer.concat([buffer, data]);
      //   while (buffer.length >= frameSize) {
      //     const frame = buffer.subarray(0, frameSize);
      //     buffer = buffer.subarray(frameSize);
      //     videoSource.onFrame({
      //       data: new Uint8Array(frame),
      //       width: 1280,
      //       height: 720,
      //     });
      //   }
      // });

      // Set the remote description
      const offer = new wrtc.RTCSessionDescription({
        type: "offer",
        sdp: data.sdp,
      });
      await peerConnection.setRemoteDescription(offer);

      const answer = await peerConnection.createAnswer();
      await peerConnection.setLocalDescription(answer);
      // console.log('Answer set successfully', answer);

      // Send answer back to client
      ws.send(
        JSON.stringify({
          ...peerConnection.localDescription,
        })
      );
      console.log("Answer sent successfully");

      // Save client info
      client = {
        id: clientId,
        ws,
        peerConnection,
        // @ts-expect-error browser and page are initialized later
        browser,
        // @ts-expect-error browser and page are initialized later
        page,
        ffmpeg,
      };
      clients.set(clientId, client);
    } else if (data.type === "answer") {
      // Handle answer from the client
      try {
        const answer = new RTCSessionDescription({
          type: "answer",
          sdp: data.sdp,
        });
        await client.peerConnection.setRemoteDescription(answer);
      } catch (error) {
        console.error("Error setting remote description:", error);
      }
    } else if (data.type === "icecandidate") {
      // Handle ICE candidates from the client
      try {
        if (!client.peerConnection) return;
        await client.peerConnection.addIceCandidate(
          new wrtc.RTCIceCandidate(data.candidate)
        );
        // console.log('ICE candidate added successfully:', data.candidate);
      } catch (error) {
        console.error("Error adding ICE candidate:", error);
      }
    } else if (data.type === "interaction") {
      // Handle interaction events from the client
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { eventType, eventData } = data;
      if (client && client.page && eventData) {
        // console.log('Page', client.page.mouse);
        try {
          // await client.page.mouse[eventType](...eventData);
        } catch (error) {
          console.error("Error handling interaction:", error);
        }
      }
    }
  });

  ws.on("close", async () => {
    console.log(`Client disconnected: ${clientId}`);
    if (client) {
      if (client.peerConnection) client.peerConnection.close();
      if (client.browser) await client.browser.close();
      if (client.ffmpeg) client.ffmpeg.kill("SIGINT");
      clients.delete(clientId);
    }
  });
});
