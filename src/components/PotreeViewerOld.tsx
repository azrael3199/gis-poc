import React, { useEffect, useRef, useState } from "react";
import useBasicViewerConfig from "../hooks/useBasicViewerConfig";
import useLoadPointcloud from "../hooks/useLoadPointCloud";

type Props = {
  pointCloudPath?: string;
};

const PotreeViewerOld = ({
  pointCloudPath = "http://localhost:3000/file/panhala/metadata.json",
}: Props) => {
  const iframe = useRef<HTMLIFrameElement>(null);
  const potreeLib = useRef(null);
  const potreeViewer = useRef(null);
  const [loaded, setLoaded] = useState(false);

  // initialize a reference to the Potree lib and viewer
  useEffect(() => {
    if (iframe.current && loaded) {
      potreeLib.current = iframe.current.contentDocument?.defaultView?.Potree;
      potreeViewer.current =
        iframe.current.contentDocument?.defaultView?.viewer;
    }
  }, []);

  // viewer config
  useBasicViewerConfig(loaded, potreeLib, potreeViewer);

  // load pointcloud
  useLoadPointcloud(
    loaded,
    potreeLib,
    potreeViewer,
    pointCloudPath,
    "Panhala",
    true
  );

  return (
    <div style={potreePointcloudStyle}>
      <iframe
        title="3D Pointcloud"
        id="potreeIframe"
        src="potree/viewer.html"
        ref={iframe}
        style={iframeStyle}
        onLoad={() => setLoaded(true)}
      />
    </div>
  );
};

const potreePointcloudStyle = {
  display: "flex",
  height: "100%",
  width: "100%",
  margin: 0,
  padding: 0,
};

const iframeStyle = {
  width: "100%",
  height: "100%",
  border: 0,
};

export default PotreeViewerOld;
