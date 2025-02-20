import React, { useEffect, useRef } from "react";

// Make sure to have a proper WebRTC adapter in your bundler;
// here we assume the browser’s native WebRTC APIs are used.

interface RTCSignalMessage {
  type: string;
  sdp?: string;
  candidate?: RTCIceCandidateInit;
  pointCloudUrl?: string;
}

interface WebRTCClientProps {
  pointCloudPath?: string;
}

const WebRTCClient: React.FC<WebRTCClientProps> = ({
  pointCloudPath = "http://localhost:3000/file/data/panhala/metadata.json",
}) => {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);

  useEffect(() => {
    const startWebRTC = async () => {
      // Close any previous connections.
      if (wsRef.current) {
        wsRef.current.close();
      }

      // Connect to the WebSocket signaling server.
      wsRef.current = new WebSocket("ws://localhost:8080/ws");

      wsRef.current.onopen = async () => {
        // Create a new RTCPeerConnection.
        const pc = new RTCPeerConnection({
          iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
        });

        // Add a transceiver for receiving video only.
        pc.addTransceiver("video", { direction: "recvonly" });

        // const transceiver = pc
        //   .getTransceivers()
        //   .find((t) => t.receiver.track.kind === "video");

        // if (transceiver) {
        //   console.log("Transceiver found:", transceiver);
        //   transceiver.setCodecPreferences([
        //     { mimeType: "video/VP8", clockRate: 90000 },
        //   ]);
        // }

        console.log("Peer connection created:", pc);

        // Create an offer.
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);

        // Send the offer message.
        if (wsRef.current) {
          wsRef.current.send(
            JSON.stringify({
              type: "offer",
              sdp: offer.sdp,
              pointCloudUrl: pointCloudPath,
            })
          );
        }

        // Handle ICE candidate events.
        pc.onicecandidate = (event) => {
          if (event.candidate && wsRef.current) {
            wsRef.current.send(
              JSON.stringify({
                type: "candidate",
                candidate: event.candidate,
              })
            );
          }
        };

        pc.addEventListener("iceconnectionstatechange", () => {
          console.log("ICE state:", pc.iceConnectionState);
        });

        // When a remote track is received, attach it to the video element.
        pc.ontrack = (event) => {
          console.log("Received remote track:", event);
          const stream = event.streams[0] || new MediaStream([event.track]);
          if (videoRef.current) {
            console.log("Attaching remote track to video element");
            videoRef.current.srcObject = stream;
            videoRef.current.play();
          }

          // Check if data is being received on the track
          const track = event.track;
          console.log(`Track added: kind=${track.kind}, id=${track.id}`);

          // Listen for mute/unmute events on the track
          track.onmute = () => {
            console.log(`Track muted: kind=${track.kind}, id=${track.id}`);
          };

          track.onunmute = () => {
            console.log(`Track unmuted: kind=${track.kind}, id=${track.id}`);
          };

          // Monitor data flow on the track
          const checkDataFlow = () => {
            if (track.readyState === "ended") {
              console.log(`Track ended: kind=${track.kind}, id=${track.id}`);
              return;
            }

            if (track.muted) {
              console.log(`Track is muted: kind=${track.kind}, id=${track.id}`);
            } else {
              console.log(
                `Track is active and receiving data: kind=${track.kind}, id=${track.id}`
              );
            }

            // Continue monitoring periodically
            setTimeout(checkDataFlow, 1000);
          };

          checkDataFlow();
        };

        // Listen for signaling messages from the server.
        wsRef.current.onmessage = async (event) => {
          console.log("Received message:", event.data);
          const data: RTCSignalMessage = JSON.parse(event.data);
          if (data.type === "answer" && data.sdp) {
            // Set remote description with the received answer.
            await pc.setRemoteDescription(
              new RTCSessionDescription({ type: "answer", sdp: data.sdp })
            );
          } else if (data.type === "candidate" && data.candidate) {
            // Add the ICE candidate.
            await pc.addIceCandidate(new RTCIceCandidate(data.candidate));
          } else {
            console.log("Unhandled message type:", data.type);
          }
        };

        pcRef.current = pc;
      };

      wsRef.current.onerror = (error) => {
        console.error("WebSocket error:", error);
      };
    };

    startWebRTC();

    // Cleanup when component unmounts.
    return () => {
      if (wsRef.current) wsRef.current.close();
      if (pcRef.current) pcRef.current.close();
    };
  }, [pointCloudPath]);

  return (
    <div className="h-full w-full">
      <h2>WebRTC Potree Stream</h2>
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        style={{ height: "100%", width: "100%", backgroundColor: "black" }}
        onPlay={(e) => console.log("Video onPlay", e)}
        onPause={(e) => console.log("Video onPause", e)}
        onEnded={(e) => console.log("Video onEnded", e)}
        onError={(e) => console.log("Video onError", e)}
        onCanPlay={(e) => console.log("Video onCanPlay", e)}
      />
    </div>
  );
};

export default WebRTCClient;
