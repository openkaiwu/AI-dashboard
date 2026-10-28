import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import "./styles.css";
import { initializeTheme } from './theme';

initializeTheme();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);

import "./foundation.css";
import "./interface.css";
import './appearance.css';
import { prepareOffline } from "./offline";
void prepareOffline();
