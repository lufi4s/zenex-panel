import { render } from "solid-js/web";
import { App } from "./App";
import { Toaster } from "./components/Toaster";
import "./index.css";

const root = document.getElementById("root");
if (!root) throw new Error("Missing #root element");

render(
  () => (
    <>
      <App />
      <Toaster />
    </>
  ),
  root,
);
