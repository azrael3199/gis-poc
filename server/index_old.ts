import express from "express";
import path from "path";
import cors from "cors";

const app = express();
const __dirname = path.resolve();

app.use(cors({ origin: "*" }));

// Endpoint to serve static files from the /data folder
app.use("/file", express.static(path.join(__dirname, "data")));

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(`Server is running on port ${PORT}`);
});
