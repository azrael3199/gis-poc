import React, { useEffect, useRef, useState } from "react";
import Hls from "hls.js";

const VideoStream = ({
  pointCloudURL = "http://localhost:8080/file/panhala/metadata.json",
}) => {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [isStreaming, setIsStreaming] = useState(false);

  const startStream = async () => {
    try {
      await fetch("http://localhost:8080/start", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          pointCloudUrl: pointCloudURL,
          viewportHeight: videoRef.current?.height || 720,
          viewportWidth: videoRef.current?.width || 1280,
        }),
      });
      setIsStreaming(true);
    } catch (error) {
      console.error("Failed to start stream:", error);
    }
  };

  const stopStream = async () => {
    try {
      await fetch("http://localhost:8080/stop", { method: "POST" });
      setIsStreaming(false);
    } catch (error) {
      console.error("Failed to stop stream:", error);
    }
  };

  useEffect(() => {
    if (videoRef.current) {
      const video = videoRef.current;
      const checkFileExists = async (url: string): Promise<boolean> => {
        try {
          const response = await fetch(url, { method: "HEAD" });
          return response.ok;
        } catch (error) {
          console.log("Error fetching file:", error);
          return false;
        }
      };

      const checkHlsFileExists = async (): Promise<boolean> => {
        const url = `http://localhost:8080/hls/output.m3u8`;
        const exists = await checkFileExists(url);
        if (!exists) {
          console.log("Waiting for HLS file to become available...");
          await new Promise((resolve) => setTimeout(resolve, 4000));
          return checkHlsFileExists();
        }
        return true;
      };

      checkHlsFileExists().then(() => {
        if (video.canPlayType("application/vnd.apple.mpegurl")) {
          video.src = `http://localhost:8080/hls/output.m3u8?nocache=${Date.now()}`;
          video.load();
          video.play();
        } else if (Hls.isSupported()) {
          const hls = new Hls();
          hls.loadSource(
            `http://localhost:8080/hls/output.m3u8?nocache=${Date.now()}`
          );
          hls.attachMedia(video);
          hls.on(Hls.Events.MANIFEST_PARSED, () => {
            video.play();
          });
        }
      });
    }
  }, [isStreaming]);

  return (
    <div className="h-screen flex flex-col">
      <h2>Live Stream</h2>
      <video
        ref={videoRef}
        controls
        autoPlay
        muted
        playsInline
        width="1920"
        height="1080"
      />
      <div>
        <button onClick={startStream} disabled={isStreaming}>
          Start Stream
        </button>
        <button onClick={stopStream} disabled={!isStreaming}>
          Stop Stream
        </button>
      </div>
    </div>
  );
};

export default VideoStream;
