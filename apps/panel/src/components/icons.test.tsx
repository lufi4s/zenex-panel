// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { render } from "solid-js/web";
import { Layers } from "./icons";

function mount(view: () => unknown) {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const dispose = render(view as () => import("solid-js").JSX.Element, host);
  return { host, dispose };
}

describe("icons", () => {
  it("draws the same icon in several places at once", () => {
    const first = mount(() => <Layers />);
    const second = mount(() => <Layers />);
    const shapes = (host: HTMLElement) => host.querySelectorAll("svg path").length;
    expect(shapes(first.host)).toBeGreaterThan(0);
    expect(shapes(second.host)).toBe(shapes(first.host));
    first.dispose();
    second.dispose();
    first.host.remove();
    second.host.remove();
  });
});
