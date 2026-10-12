import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// oxlint-disable-next-line eslint-plugin-import/no-unassigned-import -- CSS import applies the app-wide styles.
import "@/client/styles/app.css";
import { getRouter } from "./router";

const root = document.getElementById("root");
if (!root) throw new Error("Missing #root element");

createRoot(root).render(
  <StrictMode>
    <RouterProvider router={getRouter()} />
  </StrictMode>,
);
