document.addEventListener("DOMContentLoaded", function () {
  // Declare Potree viewer globally as required by Potree
  const viewer = window.viewer;

  // Get the file path for the point cloud from the query parameters
  function getQueryParameter(name) {
    const urlParams = new URLSearchParams(window.location.search);
    return urlParams.get(name);
  }

  // Get the point cloud file URL from the query parameters
  const pointcloudURL = getQueryParameter("pointcloudURL");
  const pointcloudTitle = "Viewer"; // Optional: another query parameter can be used for this
  const fitToScreen = true; // Optional: can also be passed as a query parameter if needed

  if (viewer) {
    // Apply basic viewer configuration
    useBasicViewerConfig(viewer);

    if (pointcloudURL) {
      // Load the point cloud file into the viewer
      useLoadPointcloud(viewer, pointcloudURL, pointcloudTitle, fitToScreen);
      document.getElementById("potree_render_area").onload = () => {
        const canvas = document
          .getElementById("potree_render_area")
          .querySelectorAll("canvas")[1];
        canvas.onload = () => {
          canvas.height = 1980;
          canvas.width = 1080;
          canvas.style.height = "1980px";
          canvas.style.width = "1080px";
        };
      };
    } else {
      console.error("No pointcloud URL provided in the query parameters.");
    }
  } else {
    console.error("Viewer initialization failed.");
  }

  // Function to configure the basic Potree viewer settings
  function useBasicViewerConfig(viewer) {
    viewer.setEDLEnabled(true);
    viewer.setFOV(60);
    viewer.setPointBudget(1_000_000);
    viewer.loadSettingsFromURL();
    viewer.setBackground("gradient");
    viewer.setDescription("Potree component");

    const controls = new Potree.EarthControls(viewer);
    viewer.setControls(controls);

    viewer.loadGUI(() => {
      viewer.setLanguage("en");
      console.log("Viewer GUI loaded and configured.");
    });

    console.log("Basic Potree viewer configuration applied.");
  }

  // Function to load the point cloud into the viewer
  function useLoadPointcloud(
    viewer,
    pointcloudURL,
    pointcloudTitle,
    fitToScreen = false
  ) {
    Potree.loadPointCloud(pointcloudURL, pointcloudTitle, (e) => {
      const scene = viewer.scene;
      const pointcloud = e.pointcloud;

      const material = pointcloud.material;
      material.size = 1;
      material.pointSizeType = Potree.PointSizeType.FIXED;
      material.shape = Potree.PointShape.CIRCLE;

      scene.addPointCloud(pointcloud);

      if (fitToScreen) {
        viewer.fitToScreen();
      }

      console.log(
        `Point cloud '${pointcloudTitle}' loaded from ${pointcloudURL}`
      );
    });
  }
});
