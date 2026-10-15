import { createRoot } from "react-dom/client";
import { WebApp } from "./WebApp";
import "../styles/tokens.css";
import "../styles.css";
import "./web.css";
createRoot(document.getElementById("root")!).render(<WebApp />);
